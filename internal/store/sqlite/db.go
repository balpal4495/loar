// Package sqlite provides a local SQLite-backed implementation of store.Store.
//
// This is the "operate locally" backend — no Docker, no Postgres, no network.
// Two files live in .loar/: loar.db (this) and (future) loar-graph.db (Kuzu).
//
// The schema is identical in intent to the Postgres schema. Differences:
//   - TIMESTAMPTZ → TEXT (ISO 8601 UTC)
//   - JSONB / TEXT[] → TEXT (JSON-encoded)
//   - $1 placeholders → ? placeholders
//   - FTS5 virtual table replaces Postgres GIN index
//   - WITH RECURSIVE cycle prevention via string prefix tracking
package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // register the sqlite driver
)

// DB wraps a *sql.DB and implements store.Store.
type DB struct {
	db *sql.DB
}

// New opens (or creates) the SQLite database at path and returns a DB.
func New(path string) (*DB, error) {
	// journal_mode=WAL for concurrent read access; foreign_keys enforces refs.
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_foreign_keys=on", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %s: %w", path, err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: ping %s: %w", path, err)
	}
	// SQLite performs best with a single writer; cap the pool accordingly.
	db.SetMaxOpenConns(1)
	return &DB{db: db}, nil
}

// Close closes the underlying database connection.
func (db *DB) Close() {
	db.db.Close()
}

// Migrate creates all required tables and indexes if they do not already exist.
// Safe to call on every startup — all statements are idempotent.
func (db *DB) Migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS projects (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL DEFAULT '',
			created_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS entities (
			id             TEXT PRIMARY KEY,
			project_id     TEXT NOT NULL REFERENCES projects(id),
			type           TEXT NOT NULL DEFAULT '',
			canonical_name TEXT NOT NULL,
			aliases        TEXT NOT NULL DEFAULT '[]',
			metadata       TEXT NOT NULL DEFAULT '{}',
			created_at     TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS observations (
			id          TEXT PRIMARY KEY,
			project_id  TEXT NOT NULL REFERENCES projects(id),
			source_id   TEXT NOT NULL DEFAULT '',
			content     TEXT NOT NULL,
			occurred_at TEXT,
			observed_at TEXT,
			resolved_at TEXT,
			learned_at  TEXT,
			metadata    TEXT NOT NULL DEFAULT '{}',
			created_at  TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS relationships (
			id                TEXT PRIMARY KEY,
			project_id        TEXT NOT NULL REFERENCES projects(id),
			source_id         TEXT NOT NULL,
			target_id         TEXT NOT NULL,
			relationship_type TEXT NOT NULL,
			confidence        REAL NOT NULL DEFAULT 1.0,
			created_at        TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS relationships_unique
		 ON relationships (project_id, source_id, target_id, relationship_type)`,
		`CREATE TABLE IF NOT EXISTS observation_entities (
			observation_id TEXT NOT NULL REFERENCES observations(id),
			entity_id      TEXT NOT NULL REFERENCES entities(id),
			role           TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (observation_id, entity_id)
		)`,
		// Entity confidence time-series — append-only, never mutated.
		`CREATE TABLE IF NOT EXISTS entity_confidence (
			id                TEXT PRIMARY KEY,
			entity_id         TEXT NOT NULL REFERENCES entities(id),
			project_id        TEXT NOT NULL REFERENCES projects(id),
			score             REAL NOT NULL,
			observation_count INTEGER NOT NULL DEFAULT 0,
			resolved_count    INTEGER NOT NULL DEFAULT 0,
			accuracy_rate     REAL,
			computed_at       TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS entity_confidence_entity_time
		 ON entity_confidence (entity_id, computed_at DESC)`,
		// FTS5 virtual table for full-text search on observation content.
		// id is stored alongside content so we can join back to the main table.
		`CREATE VIRTUAL TABLE IF NOT EXISTS observations_fts USING fts5(
			id UNINDEXED,
			content,
			tokenize='porter unicode61'
		)`,
	}
	for _, s := range stmts {
		if _, err := db.db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("sqlite: migrate: %w", err)
		}
	}
	return nil
}

// CleanProject deletes all observations, entities, and their links.
// The schema and project record are preserved.
func (db *DB) CleanProject(ctx context.Context) error {
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: clean: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for _, stmt := range []string{
		`DELETE FROM observation_entities`,
		`DELETE FROM entity_confidence`,
		`DELETE FROM relationships`,
		`DELETE FROM observations_fts`,
		`DELETE FROM observations`,
		`DELETE FROM entities`,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("sqlite: clean: %w", err)
		}
	}
	return tx.Commit()
}
