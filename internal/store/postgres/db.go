// Package postgres provides a PostgreSQL-backed implementation of store.Store.
package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool and implements store.Store.
type DB struct {
	pool *pgxpool.Pool
}

// New opens a connection pool to the Postgres database at dsn and returns a DB.
func New(ctx context.Context, dsn string) (*DB, error) {
	pool, err := pgxpool.New(ctx, dsn)
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
	}
	for _, s := range stmts {
		if _, err := db.pool.Exec(ctx, s); err != nil {
			return err
		}
	}
	return nil
}
