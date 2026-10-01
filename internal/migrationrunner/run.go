package migrationrunner

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"sync"

	logger "github.com/assurrussa/gologger"
	// postgres driver.
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/assurrussa/goadmin/infrastructure/outbox"
)

var gooseMu sync.Mutex

// Options describes an embedded SQL migration set.
type Options struct {
	Module    string
	TableName string
	Directory string
	Files     fs.FS
}

// Run executes an embedded SQL migration set with the shared goadmin storage config.
func Run(
	ctx context.Context,
	cfg outbox.StoragePgsqlConfig,
	db *sql.DB,
	command string,
	log logger.Logger,
	opts Options,
	args ...string,
) (errReturn error) {
	if opts.Module == "" {
		opts.Module = "goadmin"
	}
	if opts.TableName == "" {
		return errors.New("migration table name is required")
	}
	if opts.Directory == "" {
		opts.Directory = "."
	}
	if opts.Files == nil {
		return errors.New("migration files are required")
	}

	if db == nil {
		_, dsn, err := outbox.PgsqlCreateDSN(outbox.PgsqlNewOptions(
			cfg.Address,
			cfg.Username,
			cfg.Password,
			cfg.Database,
			outbox.WithPgsqlSSLMode(cfg.SSLMode),
			outbox.WithPgsqlTLSPath(cfg.TLSCert, cfg.TLSKey),
			outbox.WithPgsqlMinConnectionsCount(cfg.MinConnectionsCount),
			outbox.WithPgsqlMaxConnectionsCount(cfg.MaxConnectionsCount),
			outbox.WithPgsqlMaxConnIdleTime(cfg.MaxConnIdleTime),
			outbox.WithPgsqlMaxConnLifeTime(cfg.MaxConnLifeTime),
			outbox.WithPgsqlDebug(cfg.DebugMode),
		))
		if err != nil {
			return fmt.Errorf("create psql dsn: %w", err)
		}

		db, err = sql.Open("pgx", dsn)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer func() {
			if err := db.Close(); err != nil {
				errReturn = errors.Join(errReturn, fmt.Errorf("close database connection: %w", err))
			}
		}()
	}

	gooseMu.Lock()
	defer gooseMu.Unlock()

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	prevTableName := goose.TableName()
	goose.SetBaseFS(opts.Files)
	goose.SetTableName(opts.TableName)
	defer goose.SetBaseFS(nil)
	defer goose.SetTableName(prevTableName)

	if err := goose.RunWithOptionsContext(ctx, command, db, opts.Directory, args); err != nil {
		if errors.Is(err, goose.ErrNoMigrationFiles) {
			log.InfoContext(ctx, opts.Module+" migrations skipped", "command", command, "status", "empty")
			return nil
		}

		return fmt.Errorf("run %s migrations: %w", opts.Module, err)
	}

	return nil
}
