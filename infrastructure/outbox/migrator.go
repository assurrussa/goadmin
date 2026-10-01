package outbox

import (
	"context"
	"database/sql"

	pgsqlmigrator "github.com/assurrussa/outbox/backends/pgsql/migrator"
)

type PgsqlMigratorOption = pgsqlmigrator.Option

func PgsqlMigrate(ctx context.Context, db *sql.DB, log Logger, opts ...PgsqlMigratorOption) error {
	return pgsqlmigrator.Run(ctx, db, log, opts...)
}

func WithPgsqlMigrateCommand(command string) PgsqlMigratorOption {
	return pgsqlmigrator.WithCommand(command)
}

func WithPgsqlMigrateDirectory(directory string) PgsqlMigratorOption {
	return pgsqlmigrator.WithDirectory(directory)
}

func WithPgsqlMigrateArgs(args ...string) PgsqlMigratorOption {
	return pgsqlmigrator.WithArgs(args...)
}
