//go:build integration

package migrations_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/assurrussa/goauth/postgres"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	goadminmigrations "github.com/assurrussa/goadmin/migrations"
	"github.com/assurrussa/goadmin/tests"
)

func TestMigrateFreshResetAndLegacyRefusal(t *testing.T) {
	ctx := context.Background()
	pool, _, cleanup := tests.PrepareDB(ctx, t, "GoadminMigrationsV02")
	t.Cleanup(func() { cleanup(ctx) })
	db := sqlDB(t, pool)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	require.NoError(t, execSQL(t, ctx, db, `CREATE TABLE host_reset_sentinel (id integer PRIMARY KEY);
INSERT INTO host_reset_sentinel VALUES (1);
INSERT INTO files (original_filename, filename, slug) VALUES ('sentinel.png', 'sentinel.png', 'reset-sentinel');`))

	require.NoError(t, goadminmigrations.Reset(
		ctx,
		goadminmigrations.DatabaseConfig{},
		db,
		postgres.ConfirmResetAuthState,
	))
	requireTableMissing(t, ctx, db, "auth_subjects")
	requireTableMissing(t, ctx, db, "administrations")
	requireTableMissing(t, ctx, db, "goadmin_browser_sessions")
	requireTableMissing(t, ctx, db, "goadmin_browser_auth_state")
	var preserved int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM host_reset_sentinel").Scan(&preserved))
	require.Equal(t, 1, preserved)
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM files WHERE slug = 'reset-sentinel'").Scan(&preserved))
	require.Equal(t, 1, preserved)
	requireTable(t, ctx, db, "jobs")

	require.NoError(t, goadminmigrations.Migrate(ctx, goadminmigrations.DatabaseConfig{}, db))
	for _, table := range []string{
		"goauth_schema_version", "auth_subjects", "auth_identifiers", "auth_sessions",
		"auth_roles", "administrations", "users", "goadmin_first_admin_setup_tokens",
		"goadmin_browser_auth_state", "goadmin_browser_sessions",
	} {
		requireTable(t, ctx, db, table)
	}

	var foreignKeys int
	require.NoError(t, db.QueryRowContext(ctx, `
SELECT count(*)
FROM information_schema.table_constraints
WHERE constraint_schema = 'public'
  AND constraint_type = 'FOREIGN KEY'
  AND table_name IN ('administrations', 'users')`).Scan(&foreignKeys))
	require.Equal(t, 2, foreignKeys)

	err := goadminmigrations.Reset(
		ctx,
		goadminmigrations.DatabaseConfig{},
		db,
		postgres.ResetConfirmation("wrong"),
	)
	require.ErrorIs(t, err, postgres.ErrResetConfirmationRequired)
	requireTable(t, ctx, db, "administrations")
	requireTable(t, ctx, db, "auth_subjects")

	require.NoError(t, goadminmigrations.Reset(
		ctx,
		goadminmigrations.DatabaseConfig{},
		db,
		postgres.ConfirmResetAuthState,
	))
	_, err = db.ExecContext(ctx, `CREATE TABLE auth_subjects (
subject_id UUID PRIMARY KEY,
email TEXT NOT NULL
)`)
	require.NoError(t, err)
	err = goadminmigrations.Migrate(ctx, goadminmigrations.DatabaseConfig{}, db)
	require.True(t, errors.Is(err, postgres.ErrLegacySchemaRequiresReset), err)
	requireTableMissing(t, ctx, db, "administrations")
	requireTableMissing(t, ctx, db, "goadmin_browser_sessions")
	requireTableMissing(t, ctx, db, "goadmin_browser_auth_state")
	// Restore a complete schema for the fixture's normal down/drop cleanup, and
	// prove explicit recovery after refusing legacy auth state.
	require.NoError(t, goadminmigrations.Reset(ctx, goadminmigrations.DatabaseConfig{}, db, postgres.ConfirmResetAuthState))
	require.NoError(t, goadminmigrations.Migrate(ctx, goadminmigrations.DatabaseConfig{}, db))
}

func execSQL(t *testing.T, ctx context.Context, db *sql.DB, query string) error {
	t.Helper()
	_, err := db.ExecContext(ctx, query)
	return err
}

func sqlDB(t *testing.T, client outbox.StoragePgsqlClient) *sql.DB {
	t.Helper()
	engine, ok := client.DB().(outbox.StoragePgsqlDBPgxEnginePool)
	require.True(t, ok)

	return stdlib.OpenDBFromPool(engine.Pool())
}

func requireTable(t *testing.T, ctx context.Context, db *sql.DB, table string) {
	t.Helper()
	var name sql.NullString
	require.NoError(t, db.QueryRowContext(ctx, `SELECT to_regclass($1)::text`, "public."+table).Scan(&name))
	require.True(t, name.Valid, table)
}

func requireTableMissing(t *testing.T, ctx context.Context, db *sql.DB, table string) {
	t.Helper()
	var name sql.NullString
	require.NoError(t, db.QueryRowContext(ctx, `SELECT to_regclass($1)::text`, "public."+table).Scan(&name))
	require.False(t, name.Valid, table)
}
