package db

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Module is one module's migrations: the .sql files in FS, applied in name
// order, each once.
type Module struct {
	Name string
	FS   fs.FS
}

// Migrate applies every module's migrations that have not run yet, in the
// order given: a module after the modules it depends on. It runs as the
// pool's role, the owner of the schemas, never through the API. The
// bookkeeping lives in the shared schema.
func Migrate(ctx context.Context, pool *pgxpool.Pool, modules ...Module) error {
	if _, err := pool.Exec(ctx, `
		CREATE SCHEMA IF NOT EXISTS shared;
		CREATE TABLE IF NOT EXISTS shared.schema_migrations (
			module     text        NOT NULL,
			name       text        NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT now(),
			PRIMARY KEY (module, name)
		)`); err != nil {
		return fmt.Errorf("migrate: bookkeeping: %w", err)
	}
	for _, m := range modules {
		names, err := fs.Glob(m.FS, "*.sql")
		if err != nil {
			return err
		}
		sort.Strings(names)
		for _, name := range names {
			if err := apply(ctx, pool, m, name); err != nil {
				return err
			}
		}
	}
	return nil
}

func apply(ctx context.Context, pool *pgxpool.Pool, m Module, name string) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		// One migrator at a time, across replicas.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(7363201)"); err != nil {
			return err
		}
		var done bool
		if err := tx.QueryRow(ctx,
			"SELECT EXISTS (SELECT 1 FROM shared.schema_migrations WHERE module = $1 AND name = $2)",
			m.Name, name).Scan(&done); err != nil {
			return err
		}
		if done {
			return nil
		}
		sql, err := fs.ReadFile(m.FS, name)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("migrate %s/%s: %w", m.Name, path.Base(name), err)
		}
		_, err = tx.Exec(ctx, "INSERT INTO shared.schema_migrations (module, name) VALUES ($1, $2)", m.Name, name)
		return err
	})
}
