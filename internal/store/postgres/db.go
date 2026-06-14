// Package postgres provides a PostgreSQL-backed implementation of store.Store.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool and implements store.Store.
type DB struct {
	pool *pgxpool.Pool
}

// New opens a connection pool to the Postgres database at dsn and returns a DB.
// search_path is set to ag_catalog first so Apache AGE's cypher() function is
// always resolvable without schema qualification.
func New(ctx context.Context, dsn string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	// Apache AGE functions live in ag_catalog. Setting it first in search_path
	// makes cypher() and agtype available without full qualification.
	cfg.ConnConfig.RuntimeParams["search_path"] = `ag_catalog, "$user", public`
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &DB{pool: pool}, nil
}

// Close releases all connections in the pool.
func (db *DB) Close() {
	db.pool.Close()
}

// Migrate runs DDL statements to create all required tables if they do not
// already exist. Safe to call on every startup.
func (db *DB) Migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS projects (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL DEFAULT '',
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS entities (
			id             TEXT PRIMARY KEY,
			project_id     TEXT NOT NULL REFERENCES projects(id),
			type           TEXT NOT NULL DEFAULT '',
			canonical_name TEXT NOT NULL,
			aliases        TEXT[] NOT NULL DEFAULT '{}',
			metadata       JSONB NOT NULL DEFAULT '{}',
			created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS observations (
			id          TEXT PRIMARY KEY,
			project_id  TEXT NOT NULL REFERENCES projects(id),
			source_id   TEXT NOT NULL DEFAULT '',
			content     TEXT NOT NULL,
			occurred_at TIMESTAMPTZ,
			observed_at TIMESTAMPTZ,
			resolved_at TIMESTAMPTZ,
			learned_at  TIMESTAMPTZ,
			metadata    JSONB NOT NULL DEFAULT '{}',
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS relationships (
			id                TEXT PRIMARY KEY,
			project_id        TEXT NOT NULL REFERENCES projects(id),
			source_id         TEXT NOT NULL,
			target_id         TEXT NOT NULL,
			relationship_type TEXT NOT NULL,
			confidence        DOUBLE PRECISION NOT NULL DEFAULT 1.0,
			created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS observation_entities (
			observation_id TEXT NOT NULL REFERENCES observations(id),
			entity_id      TEXT NOT NULL REFERENCES entities(id),
			role           TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (observation_id, entity_id)
		)`,
		// Full-text search index on observation content.
		`CREATE INDEX IF NOT EXISTS observations_content_fts
		 ON observations USING gin(to_tsvector('english', content))`,
		// Unique constraint on relationships so co_occurs links are idempotent.
		`CREATE UNIQUE INDEX IF NOT EXISTS relationships_unique
		 ON relationships (project_id, source_id, target_id, relationship_type)`,
		// Entity confidence time-series — one row per entity per learn run.
		// Never mutated; trend is queryable by ordering on computed_at.
		`CREATE TABLE IF NOT EXISTS entity_confidence (
			id                TEXT PRIMARY KEY,
			entity_id         TEXT NOT NULL REFERENCES entities(id),
			project_id        TEXT NOT NULL REFERENCES projects(id),
			score             DOUBLE PRECISION NOT NULL,
			observation_count INTEGER NOT NULL DEFAULT 0,
			resolved_count    INTEGER NOT NULL DEFAULT 0,
			accuracy_rate     DOUBLE PRECISION,
			computed_at       TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS entity_confidence_entity_time
		 ON entity_confidence (entity_id, computed_at DESC)`,
		// Apache AGE graph extension.
		`CREATE EXTENSION IF NOT EXISTS age`,
		// Create the loar property graph if it doesn't exist yet.
		// ag_catalog.create_graph errors if the graph already exists, so we
		// guard with an existence check.
		`DO $$ BEGIN
		   IF NOT EXISTS (SELECT 1 FROM ag_catalog.ag_graph WHERE name = 'loar_graph') THEN
		     PERFORM ag_catalog.create_graph('loar_graph');
		   END IF;
		 END $$`,
	}
	for _, s := range stmts {
		if _, err := db.pool.Exec(ctx, s); err != nil {
			return err
		}
	}
	return db.syncToGraph(ctx)
}

// syncToGraph populates the Apache AGE property graph from the existing
// relational tables. Uses MERGE so it is safe to call on every startup —
// no duplicates are created. Existing projects are automatically indexed
// without requiring a manual re-ingest.
func (db *DB) syncToGraph(ctx context.Context) error {
	// Sync entities.
	rows, err := db.pool.Query(ctx,
		`SELECT id, project_id, canonical_name, type FROM entities`)
	if err != nil {
		return fmt.Errorf("sync graph entities: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, projectID, canonicalName, typ string
		if err := rows.Scan(&id, &projectID, &canonicalName, &typ); err != nil {
			return err
		}
		params := fmt.Sprintf(`{"id": %q, "project_id": %q, "canonical_name": %q, "type": %q}`,
			id, projectID, canonicalName, typ)
		if _, err := db.pool.Exec(ctx,
			`SELECT * FROM cypher('loar_graph', $$
			   MERGE (n:Entity {id: $id})
			   ON CREATE SET n.project_id = $project_id, n.canonical_name = $canonical_name, n.type = $type
			   ON MATCH SET  n.project_id = $project_id, n.canonical_name = $canonical_name, n.type = $type
			 $$, $1::agtype) AS (v agtype)`, params); err != nil {
			return fmt.Errorf("sync graph entity %s: %w", id, err)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Sync relationships.
	relRows, err := db.pool.Query(ctx,
		`SELECT id, source_id, target_id, relationship_type, confidence FROM relationships`)
	if err != nil {
		return fmt.Errorf("sync graph relationships: %w", err)
	}
	defer relRows.Close()
	for relRows.Next() {
		var id, sourceID, targetID, relType string
		var confidence float64
		if err := relRows.Scan(&id, &sourceID, &targetID, &relType, &confidence); err != nil {
			return err
		}
		params := fmt.Sprintf(`{"id": %q, "source_id": %q, "target_id": %q, "rel_type": %q, "confidence": %v}`,
			id, sourceID, targetID, relType, confidence)
		// MATCH may return 0 rows if entities haven't been synced yet — that is
		// fine; the relationship will be written on next CreateRelationship call.
		if _, err := db.pool.Exec(ctx,
			`SELECT * FROM cypher('loar_graph', $$
			   MATCH (a:Entity {id: $source_id}), (b:Entity {id: $target_id})
			   MERGE (a)-[r:RELATED {id: $id}]->(b)
			   ON CREATE SET r.type = $rel_type, r.confidence = $confidence
			   ON MATCH SET  r.confidence = $confidence
			 $$, $1::agtype) AS (v agtype)`, params); err != nil {
			// Non-fatal: relationship will be re-synced when entities exist.
			continue
		}
	}
	return relRows.Err()
}

// CleanProject deletes all observations, entities, and their links from the
// database. The schema and project record are preserved. Runs in a single
// transaction so a failure leaves the DB unchanged.
func (db *DB) CleanProject(ctx context.Context) error {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("clean: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for _, stmt := range []string{
		`DELETE FROM observation_entities`,
		`DELETE FROM entity_confidence`,
		`DELETE FROM relationships`,
		`DELETE FROM observations`,
		`DELETE FROM entities`,
	} {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("clean: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	// Wipe all Entity nodes from the AGE graph. Must happen outside the
	// transaction above because AGE DDL operations use their own internal
	// transaction management.
	if _, err := db.pool.Exec(ctx,
		`SELECT * FROM cypher('loar_graph', $$
		   MATCH (n:Entity) DETACH DELETE n
		 $$) AS (v agtype)`); err != nil {
		return fmt.Errorf("clean: wipe graph: %w", err)
	}
	return nil
}
