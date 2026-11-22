package storage

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// RunMigrations aplica todas as migrations *.up.sql embutidas em MigrationsFS
// em ordem lexicográfica, registrando cada versão aplicada na tabela schema_migrations.
func RunMigrations(ctx context.Context, db *sql.DB) error {
	const createTableSQL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     TEXT PRIMARY KEY,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);`
	if _, err := db.ExecContext(ctx, createTableSQL); err != nil {
		return fmt.Errorf("erro ao criar tabela schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(MigrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("erro ao listar migrations embutidas: %w", err)
	}

	var upFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			upFiles = append(upFiles, e.Name())
		}
	}
	sort.Strings(upFiles)

	for _, filename := range upFiles {
		version := strings.TrimSuffix(filename, ".up.sql")

		var exists bool
		err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&exists)
		if err != nil {
			return fmt.Errorf("erro ao verificar migration %s: %w", version, err)
		}
		if exists {
			continue
		}

		sqlBytes, err := MigrationsFS.ReadFile(path.Join("migrations", filename))
		if err != nil {
			return fmt.Errorf("erro ao ler migration %s: %w", filename, err)
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("erro ao iniciar transação da migration %s: %w", version, err)
		}

		if _, err := tx.ExecContext(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("erro ao executar migration %s: %w", version, err)
		}

		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("erro ao registrar migration %s: %w", version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("erro ao commitar migration %s: %w", version, err)
		}
	}

	return nil
}
