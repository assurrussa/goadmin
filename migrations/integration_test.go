//go:build integration

package migrations_test

import (
	"context"
	"database/sql"
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
	requireTableMissing(t, ctx, db, "goadmin_admin_preferences")
	requireTableMissing(t, ctx, db, "goadmin_branding")
	requireTableMissing(t, ctx, db, "admin_profile_presentations")
	requireTableMissing(t, ctx, db, "admin_admission_events")
	requireTableMissing(t, ctx, db, "admin_admission_invitations")
	requireTableMissing(t, ctx, db, "admin_admissions")
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
		"admin_admissions", "admin_admission_invitations", "admin_admission_events", "admin_profile_presentations", "goadmin_admin_preferences", "goadmin_branding",
	} {
		requireTable(t, ctx, db, table)
	}

	// The public facade must install the pinned GoAuth schema, not leave a
	// successful-looking host on its previous notification/step-up schema.
	var authVersion int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT max(version) FROM goauth_schema_version`).Scan(&authVersion))
	require.Equal(t, 16, authVersion)
	var stepUpColumns int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.columns
WHERE table_schema='public' AND table_name='auth_oidc_authorization_requests'
AND column_name IN ('step_up_session','step_up_binding','step_up_session_id','step_up_cookie_digest','step_up_verified_at')`).Scan(&stepUpColumns))
	require.Equal(t, 5, stepUpColumns)

	// Additive gradient storage keeps existing installations solid by default.
	var accentEnd string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT accent_color_end FROM goadmin_branding WHERE singleton=true`).Scan(&accentEnd))
	require.Empty(t, accentEnd)
	var nullable, columnDefault string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT is_nullable,column_default FROM information_schema.columns WHERE table_name='goadmin_branding' AND column_name='accent_color_end'`).Scan(&nullable, &columnDefault))
	require.Equal(t, "NO", nullable)
	require.Contains(t, columnDefault, "''")

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
	requireTable(t, ctx, db, "admin_profile_presentations")
	requireTable(t, ctx, db, "admin_admissions")
	requireTable(t, ctx, db, "admin_admission_invitations")
	requireTable(t, ctx, db, "admin_admission_events")
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
	require.ErrorIs(t, err, postgres.ErrLegacySchemaRequiresReset)
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
