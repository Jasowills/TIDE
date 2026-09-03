package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Migrator applies embedded SQL migrations in lexical order and records them
// in schema_migrations. `tide doctor` reports version + pending count (T004).
type Migrator struct {
	DSN string
}

func (m Migrator) open() (*sql.DB, error) {
	db, err := sql.Open("pgx", m.DSN)
	if err != nil {
		return nil, fmt.Errorf("db: open: %w", err)
	}
	return db, nil
}

// Pending returns migration filenames not yet recorded in schema_migrations.
func (m Migrator) Pending(ctx context.Context) ([]string, error) {
	db, err := m.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("db: read migrations: %w", err)
	}
	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version BIGINT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return nil, fmt.Errorf("db: ensure schema_migrations: %w", err)
	}
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("db: read schema_migrations: %w", err)
	}
	defer rows.Close()
	applied := map[string]bool{}
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[fmt.Sprintf("%04d", v)] = true
	}
	var pending []string
	for _, f := range files {
		prefix := strings.SplitN(f, "_", 2)[0]
		if !applied[prefix] {
			pending = append(pending, f)
		}
	}
	return pending, nil
}

// ApplyPending applies pending migrations in order and records versions.
// Idempotent: applied versions are skipped via schema_migrations.
func (m Migrator) ApplyPending(ctx context.Context) ([]string, error) {
	db, err := m.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	pending, err := m.Pending(ctx)
	if err != nil {
		return nil, err
	}
	var applied []string
	for _, f := range pending {
		raw, err := migrationFS.ReadFile("migrations/" + f)
		if err != nil {
			return applied, fmt.Errorf("db: read %s: %w", f, err)
		}
		if _, err := db.ExecContext(ctx, string(raw)); err != nil {
			return applied, fmt.Errorf("db: apply %s: %w", f, err)
		}
		version := strings.SplitN(f, "_", 2)[0]
		var v int64
		if _, err := fmt.Sscanf(version, "%d", &v); err != nil {
			return applied, fmt.Errorf("db: bad version in %s", f)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO schema_migrations(version) VALUES ($1) ON CONFLICT DO NOTHING`, v); err != nil {
			return applied, fmt.Errorf("db: record %s: %w", f, err)
		}
		applied = append(applied, f)
	}
	return applied, nil
}
func (m Migrator) Version(ctx context.Context) (int64, error) {
	db, err := m.open()
	if err != nil {
		return 0, err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version BIGINT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return 0, err
	}
	var v sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT max(version) FROM schema_migrations`).Scan(&v); err != nil {
		return 0, fmt.Errorf("db: version: %w", err)
	}
	if !v.Valid {
		return 0, nil
	}
	return v.Int64, nil
}
