package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/assurrussa/goauth/postgres"
	logger "github.com/assurrussa/gologger"
	_ "github.com/jackc/pgx/v5/stdlib" // register pgx for the public *sql.DB migration entrypoint

	"github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/internal/connectionconfig"
)

// DatabaseConfig is the stable public PostgreSQL config accepted by goadmin migrations.
type DatabaseConfig struct {
	DSN                 string
	Address             string
	Username            string
	Password            string
	Database            string
	SSLMode             string
	DebugMode           bool
	MinConnectionsCount int32
	MaxConnectionsCount int32
	TLSCert             string
	TLSKey              string
	MaxConnIdleTime     time.Duration
	MaxConnLifeTime     time.Duration
}

// Migrate installs canonical goauth storage, the goadmin host schema, and
// canonical gouploads lifecycle storage in that order. Legacy goauth v0.1
// schemas are rejected by the canonical runner and are never reset implicitly.
func Migrate(ctx context.Context, cfg DatabaseConfig, db *sql.DB) error {
	return withDatabase(ctx, cfg, db, func(database *sql.DB, storageCfg outbox.StoragePgsqlConfig) error {
		if err := postgres.Migrate(ctx, database); err != nil {
			return fmt.Errorf("migrate canonical goauth schema: %w", err)
		}
		if err := run(ctx, storageCfg, database, "up", logger.Discard()); err != nil {
			return fmt.Errorf("migrate goadmin schema: %w", err)
		}
		if err := migrateUploads(ctx, database); err != nil {
			return fmt.Errorf("migrate canonical gouploads schema: %w", err)
		}

		return nil
	})
}

// Reset removes goadmin auth projections before resetting canonical auth state.
// Upload data and its migration history are preserved. The typed confirmation
// prevents an accidental production-data deletion.
func Reset(
	ctx context.Context,
	cfg DatabaseConfig,
	db *sql.DB,
	confirmation postgres.ResetConfirmation,
) error {
	if confirmation != postgres.ConfirmResetAuthState {
		return postgres.ErrResetConfirmationRequired
	}
	return withDatabase(ctx, cfg, db, func(database *sql.DB, _ outbox.StoragePgsqlConfig) error {
		if err := resetAuthProjections(ctx, database); err != nil {
			return fmt.Errorf("reset goadmin schema: %w", err)
		}
		if err := postgres.Down(ctx, database, confirmation); err != nil {
			return fmt.Errorf("reset canonical goauth schema: %w", err)
		}

		return nil
	})
}

func resetAuthProjections(ctx context.Context, db *sql.DB) error {
	const statement = `
DROP TABLE IF EXISTS goadmin_browser_sessions CASCADE;
DROP TABLE IF EXISTS goadmin_browser_auth_state CASCADE;
DROP TABLE IF EXISTS admin_notifications CASCADE;
DROP TABLE IF EXISTS goadmin_first_admin_setup_tokens CASCADE;
DROP TABLE IF EXISTS users CASCADE;
DROP TABLE IF EXISTS administrations CASCADE;
DROP TABLE IF EXISTS goadmin_users_goose_db_version CASCADE;
DROP TABLE IF EXISTS goadmin_goose_db_version CASCADE;`

	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin goadmin auth projection reset: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("drop goadmin auth projections: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit goadmin auth projection reset: %w", err)
	}

	return nil
}

func withDatabase(
	ctx context.Context,
	cfg DatabaseConfig,
	db *sql.DB,
	fn func(*sql.DB, outbox.StoragePgsqlConfig) error,
) (returnErr error) {
	if db != nil {
		return fn(db, outbox.StoragePgsqlConfig{})
	}
	storageCfg, err := storageConfig(cfg)
	if err != nil {
		return err
	}
	_, dsn, err := outbox.PgsqlCreateDSN(outbox.PgsqlNewOptions(
		storageCfg.Address,
		storageCfg.Username,
		storageCfg.Password,
		storageCfg.Database,
		outbox.WithPgsqlSSLMode(storageCfg.SSLMode),
		outbox.WithPgsqlTLSPath(storageCfg.TLSCert, storageCfg.TLSKey),
		outbox.WithPgsqlMinConnectionsCount(storageCfg.MinConnectionsCount),
		outbox.WithPgsqlMaxConnectionsCount(storageCfg.MaxConnectionsCount),
		outbox.WithPgsqlMaxConnIdleTime(storageCfg.MaxConnIdleTime),
		outbox.WithPgsqlMaxConnLifeTime(storageCfg.MaxConnLifeTime),
		outbox.WithPgsqlDebug(storageCfg.DebugMode),
	))
	if err != nil {
		return fmt.Errorf("create goadmin migration DSN: %w", err)
	}
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open goadmin migration database: %w", err)
	}
	defer func() {
		if closeErr := database.Close(); closeErr != nil {
			returnErr = errors.Join(returnErr, closeErr)
		}
	}()
	if err := database.PingContext(ctx); err != nil {
		return fmt.Errorf("ping goadmin migration database: %w", err)
	}

	return fn(database, storageCfg)
}

func storageConfig(cfg DatabaseConfig) (outbox.StoragePgsqlConfig, error) {
	return connectionconfig.PgsqlStorageConfig(connectionconfig.PgsqlConfig{
		DSN:                 cfg.DSN,
		Address:             cfg.Address,
		Username:            cfg.Username,
		Password:            cfg.Password,
		Database:            cfg.Database,
		SSLMode:             cfg.SSLMode,
		DebugMode:           cfg.DebugMode,
		MinConnectionsCount: cfg.MinConnectionsCount,
		MaxConnectionsCount: cfg.MaxConnectionsCount,
		TLSCert:             cfg.TLSCert,
		TLSKey:              cfg.TLSKey,
		MaxConnIdleTime:     cfg.MaxConnIdleTime,
		MaxConnLifeTime:     cfg.MaxConnLifeTime,
	})
}
