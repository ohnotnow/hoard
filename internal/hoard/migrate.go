package hoard

import (
	"context"
	"database/sql"
	"fmt"
)

type migration struct {
	version     int
	description string
	apply       func(ctx context.Context, tx *sql.Tx) error
}

var migrations = []migration{
	{1, "baseline schema", func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			CREATE TABLE sources (
				id INTEGER PRIMARY KEY,
				label TEXT NOT NULL UNIQUE,
				ingested_at TEXT NOT NULL
			);
			CREATE TABLE titles (
				id INTEGER PRIMARY KEY,
				key TEXT NOT NULL UNIQUE,
				display TEXT NOT NULL
			);
			CREATE TABLE files (
				id INTEGER PRIMARY KEY,
				source_id INTEGER NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
				path TEXT NOT NULL,
				ext TEXT NOT NULL,
				media_type TEXT NOT NULL CHECK (media_type IN ('video', 'audio', 'other')),
				title_id INTEGER REFERENCES titles(id)
			);
			CREATE INDEX idx_files_source ON files(source_id);
			CREATE INDEX idx_files_title ON files(title_id);
			CREATE TABLE aliases (
				id INTEGER PRIMARY KEY,
				title_id INTEGER NOT NULL REFERENCES titles(id) ON DELETE CASCADE,
				alias_key TEXT NOT NULL UNIQUE
			);
		`)
		return err
	}},
	// Append new migrations here. Never modify existing ones.
}

func runMigrations(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_version (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			version INTEGER NOT NULL DEFAULT 0
		);
		INSERT OR IGNORE INTO schema_version (id, version) VALUES (1, 0);
	`)
	if err != nil {
		return fmt.Errorf("bootstrap schema_version: %w", err)
	}

	var current int
	if err := db.QueryRowContext(ctx, "SELECT version FROM schema_version WHERE id = 1").Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if err := m.apply(ctx, tx); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d (%s): %w", m.version, m.description, err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE schema_version SET version = ? WHERE id = 1", m.version); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d (%s): record version: %w", m.version, m.description, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
