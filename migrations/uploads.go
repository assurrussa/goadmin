package migrations

import (
	"context"
	"database/sql"
	"fmt"

	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

const (
	uploadsTableName = "goadmin_uploads_goose_db_version"
	// Core already owns this table and its indexes. All later migrations remain
	// upstream-owned, including versions that collide with the core ledger.
	uploadsFilesMigration = "20250705111436_create_files_table.sql"
)

func migrateUploads(ctx context.Context, db *sql.DB) error {
	provider, err := uploadsProvider(db)
	if err != nil {
		return err
	}
	// The provider borrows db; closing it would close the host's database.
	_, err = provider.Up(ctx)
	return err
}

func uploadsProvider(db *sql.DB) (*goose.Provider, error) {
	files, err := uploadhost.MigrationsFS()
	if err != nil {
		return nil, fmt.Errorf("load gouploads migration files: %w", err)
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, fmt.Errorf("create gouploads migration lock: %w", err)
	}
	return goose.NewProvider(goose.DialectPostgres, db, files,
		goose.WithTableName(uploadsTableName),
		goose.WithExcludeNames([]string{uploadsFilesMigration}),
		goose.WithDisableGlobalRegistry(true),
		goose.WithSessionLocker(locker),
	)
}
