package migrations

import (
	"context"
	"database/sql"
	"embed"

	logger "github.com/assurrussa/gologger"

	"github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/internal/migrationrunner"
)

const (
	// TableName is the default goose table for goadmin core migrations.
	TableName = "goadmin_goose_db_version"
	directory = "sql"
)

//go:embed sql/*.sql
var files embed.FS

func run(
	ctx context.Context,
	cfg outbox.StoragePgsqlConfig,
	db *sql.DB,
	command string,
	log logger.Logger,
	args ...string,
) error {
	return migrationrunner.Run(ctx, cfg, db, command, log, migrationrunner.Options{
		Module:    "goadmin core",
		TableName: TableName,
		Directory: directory,
		Files:     files,
	}, args...)
}
