package storage

import "embed"

// MigrationsFS embute os arquivos SQL versionados (.up.sql e .down.sql) no binário.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS
