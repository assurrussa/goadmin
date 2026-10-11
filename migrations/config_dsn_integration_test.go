//go:build integration

package migrations //nolint:testpackage // observes the exact owned migration connection.

import (
	"context"
	"database/sql"
	"net/url"
	"testing"
	"time"

	"github.com/assurrussa/goauth/postgres"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/tests"
)

func TestWithDatabasePreservesOwnedDSNRuntimeParams(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	base, _, cleanup := tests.PrepareDB(ctx, t, "MigrationDSNRuntimeParams", tests.WithDatabasePathFilesMigration())
	t.Cleanup(func() { cleanup(context.Background()) })
	borrowed := stdlib.OpenDBFromPool(base.DB().Pool())
	t.Cleanup(func() { require.NoError(t, borrowed.Close()) })
	t.Cleanup(func() {
		require.NoError(t, Reset(context.Background(), DatabaseConfig{}, borrowed, postgres.ConfirmResetAuthState))
	})
	parsed, err := url.Parse(base.DB().Pool().Config().ConnString())
	require.NoError(t, err)
	query := parsed.Query()
	query.Set("application_name", "goadmin dsn migration")
	query.Set("TimeZone", "UTC")
	query.Set("statement_timeout", "7s")
	query.Set("lock_timeout", "2s")
	query.Set("idle_in_transaction_session_timeout", "30s")
	query.Set("search_path", "public")
	parsed.RawQuery = query.Encode()
	cfg := DatabaseConfig{DSN: parsed.String()}
	var owned *sql.DB
	err = withDatabase(ctx, cfg, nil, func(db *sql.DB, _ outbox.StoragePgsqlConfig) error {
		owned = db
		var app, zone, statement, lock, idle, schema string
		require.NoError(t, db.QueryRowContext(ctx, `
SELECT current_setting('application_name'), current_setting('TimeZone'),
       current_setting('statement_timeout'), current_setting('lock_timeout'),
       current_setting('idle_in_transaction_session_timeout'), current_setting('search_path')`).
			Scan(&app, &zone, &statement, &lock, &idle, &schema))
		require.Equal(t, "goadmin dsn migration", app)
		require.Equal(t, "UTC", zone)
		require.Equal(t, "7s", statement)
		require.Equal(t, "2s", lock)
		require.Equal(t, "30s", idle)
		require.Equal(t, "public", schema)
		return nil
	})
	require.NoError(t, err)
	require.NotNil(t, owned)
	require.ErrorContains(t, owned.PingContext(ctx), "database is closed")
	require.NoError(t, Migrate(ctx, cfg, nil))
	var table sql.NullString
	require.NoError(t, borrowed.QueryRowContext(ctx, "SELECT to_regclass('public.goadmin_browser_sessions')::text").Scan(&table))
	require.True(t, table.Valid)
	require.NoError(t, borrowed.QueryRowContext(ctx, "SELECT to_regclass('public.upload_sessions')::text").Scan(&table))
	require.True(t, table.Valid)
}
