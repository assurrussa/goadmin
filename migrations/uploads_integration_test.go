//go:build integration

package migrations //nolint:testpackage // verifies the exact previous public migration sequence

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/assurrussa/goauth/postgres"
	logger "github.com/assurrussa/gologger"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/tests"
)

func TestUploadsMigrateFreshRepeatAndResetPreservesLifecycle(t *testing.T) {
	ctx, db := uploadMigrationDatabase(t, "UploadMigrationsFresh")
	// The fixture installs GoAuth only. Remove it to exercise the entire public
	// facade from an empty database, without a handwritten upload schema.
	require.NoError(t, postgres.Down(ctx, db, postgres.ConfirmResetAuthState))
	require.NoError(t, Migrate(ctx, DatabaseConfig{}, db))
	assertUploadSchema(t, ctx, db)
	require.NoError(t, Migrate(ctx, DatabaseConfig{}, db))
	assertUploadLedger(t, ctx, db)

	_, err := db.ExecContext(ctx, `
INSERT INTO files (original_filename, filename, slug, file_type, mime_type)
VALUES ('preserved.wav', 'preserved.wav', 'preserved-audio', 7, 'audio/wav');
INSERT INTO upload_sessions (id, upload_length, object_path, original_name, file_name,
 owner_uuid, multipart_upload_id, status, finalization_key, expires_at)
VALUES ('00000000-0000-4000-8000-000000000001', 10, 'preserved/session', 'preserved.wav',
 'preserved.wav', '00000000-0000-4000-8000-000000000002', 'multipart', 'cleaning',
 '00000000-0000-4000-8000-000000000003', clock_timestamp() + interval '1 day');
INSERT INTO upload_finalizations (finalization_key, binding_hash, file_id)
SELECT '00000000-0000-4000-8000-000000000003', repeat('a', 64), id FROM files WHERE slug = 'preserved-audio';
INSERT INTO file_deletions (file_id, payload)
SELECT id, '{}'::jsonb FROM files WHERE slug = 'preserved-audio';`)
	require.NoError(t, err)

	require.NoError(t, Reset(ctx, DatabaseConfig{}, db, postgres.ConfirmResetAuthState))
	assertUploadLedger(t, ctx, db)
	assertUploadLifecycleRows(t, ctx, db)
	require.NoError(t, Migrate(ctx, DatabaseConfig{}, db))
	assertUploadSchema(t, ctx, db)
	assertUploadLedger(t, ctx, db)
	assertUploadLifecycleRows(t, ctx, db)
}

// The host-managed facade intentionally has the same auth/core behavior as the
// pre-v0.9.1 facade. Upload execution and ledger ownership remain the caller's.
func TestHostManagedUploadsMigrateDoesNotCreateOrAdoptUploadHistory(t *testing.T) {
	ctx, db := uploadMigrationDatabase(t, "HostManagedUploads")
	require.NoError(t, postgres.Down(ctx, db, postgres.ConfirmResetAuthState))
	require.NoError(t, MigrateWithHostManagedUploads(ctx, DatabaseConfig{}, db))
	var auth, core, uploadLedger, sessions bool
	require.NoError(t, db.QueryRowContext(ctx, `SELECT
 to_regclass('public.auth_subjects') IS NOT NULL,
 to_regclass('public.goadmin_browser_sessions') IS NOT NULL,
 to_regclass('public.goadmin_uploads_goose_db_version') IS NOT NULL,
 to_regclass('public.upload_sessions') IS NOT NULL`).Scan(&auth, &core, &uploadLedger, &sessions))
	require.True(t, auth)
	require.True(t, core)
	require.False(t, uploadLedger)
	require.False(t, sessions, "the caller, not this facade, installs upload migrations")
	require.NoError(t, MigrateWithHostManagedUploads(ctx, DatabaseConfig{}, db))
	// The unchanged default remains fully managed and installs the upload pack.
	require.NoError(t, Migrate(ctx, DatabaseConfig{}, db))
	assertUploadSchema(t, ctx, db)
	assertUploadLedger(t, ctx, db)
}

func TestUploadsMigrateExistingGoAdminPreservesFiles(t *testing.T) {
	ctx, db := uploadMigrationDatabase(t, "UploadMigrationsUpgrade")
	// Reproduce the previous public facade exactly: canonical GoAuth (fixture)
	// followed by core GoAdmin SQL under its real ledger, without GoUploads.
	require.NoError(t, run(ctx, outbox.StoragePgsqlConfig{}, db, "up", logger.Discard()))
	_, err := db.ExecContext(ctx, `
CREATE TABLE host_upload_upgrade_sentinel (id integer PRIMARY KEY);
INSERT INTO host_upload_upgrade_sentinel VALUES (42);
INSERT INTO files (id, original_filename, filename, slug, file_type, mime_type, data)
VALUES (101, 'old.png', 'old.png', 'existing-image', 0, 'image/png', '{"width": 5}'),
       (102, 'old.wav', 'old.wav', 'existing-audio', 7, 'audio/wav', '{}');`)
	require.NoError(t, err)
	require.NoError(t, Migrate(ctx, DatabaseConfig{}, db))
	require.NoError(t, Migrate(ctx, DatabaseConfig{}, db))
	assertUploadSchema(t, ctx, db)
	assertUploadLedger(t, ctx, db)
	var fileType, width, sentinel int
	require.NoError(t, db.QueryRowContext(ctx, `
SELECT file_type, (data->>'width')::int FROM files WHERE id=101 AND slug='existing-image'`).Scan(&fileType, &width))
	require.Equal(t, 1, fileType)
	require.Equal(t, 5, width)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT file_type FROM files WHERE id=102 AND slug='existing-audio'`).Scan(&fileType))
	require.Equal(t, 7, fileType)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT id FROM host_upload_upgrade_sentinel`).Scan(&sentinel))
	require.Equal(t, 42, sentinel)
}

func TestUploadsMigrateDuplicatePrimaryRollsBackAndCanRetry(t *testing.T) {
	ctx, db := uploadMigrationDatabase(t, "UploadMigrationsPrimaryConflict")
	require.NoError(t, run(ctx, outbox.StoragePgsqlConfig{}, db, "up", logger.Discard()))
	_, err := db.ExecContext(ctx, `
INSERT INTO files (original_filename, filename, slug, object_type, object_id, is_primary, mime_type)
VALUES ('one.png', 'one.png', 'conflict-one', 'host-object', 1, true, 'image/png'),
       ('two.png', 'two.png', 'conflict-two', 'host-object', 1, true, 'image/png');`)
	require.NoError(t, err)
	err = Migrate(ctx, DatabaseConfig{}, db)
	require.ErrorContains(t, err, "migrate canonical gouploads schema")
	require.ErrorContains(t, err, "duplicate primary files")
	var name sql.NullString
	require.NoError(t, db.QueryRowContext(ctx, `SELECT to_regclass('public.upload_finalizations')::text`).Scan(&name))
	require.False(t, name.Valid, "the lifecycle migration must roll back its DDL")
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM files WHERE file_type=0 AND is_primary`).Scan(&count))
	require.Equal(t, 2, count, "the lifecycle migration must roll back its data backfill")
	require.NoError(t, db.QueryRowContext(ctx, `
SELECT count(*) FROM goadmin_uploads_goose_db_version WHERE version_id=20260930120000 AND is_applied`).Scan(&count))
	require.Zero(t, count)
	_, err = db.ExecContext(ctx, `UPDATE files SET is_primary=false WHERE slug='conflict-two'`)
	require.NoError(t, err)
	require.NoError(t, Migrate(ctx, DatabaseConfig{}, db))
	assertUploadSchema(t, ctx, db)
	assertUploadLedger(t, ctx, db)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM files WHERE file_type=1`).Scan(&count))
	require.Equal(t, 2, count)
}

func uploadMigrationDatabase(t *testing.T, name string) (context.Context, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	pool, _, cleanup := tests.PrepareDB(ctx, t, name, tests.WithDatabasePathFilesMigration())
	t.Cleanup(func() { cleanup(context.Background()) })
	engine, ok := pool.DB().(outbox.StoragePgsqlDBPgxEnginePool)
	require.True(t, ok)
	db := stdlib.OpenDBFromPool(engine.Pool())
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	t.Cleanup(func() {
		require.NoError(t, Reset(context.Background(), DatabaseConfig{}, db, postgres.ConfirmResetAuthState))
	})
	return ctx, db
}

func assertUploadSchema(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	for _, table := range []string{
		"auth_subjects", "goadmin_browser_sessions", "jobs", "files",
		"upload_sessions", "upload_finalizations", "file_deletions",
		"files_primary_unique_idx", "upload_sessions_expiry_all_idx",
	} {
		var name sql.NullString
		require.NoError(t, db.QueryRowContext(ctx, `SELECT to_regclass($1)::text`, "public."+table).Scan(&name))
		require.True(t, name.Valid, table)
	}
}

func assertUploadLedger(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `
SELECT count(*) FROM goadmin_uploads_goose_db_version WHERE version_id>0 AND is_applied`).Scan(&count))
	require.Equal(t, 2, count)
	for _, version := range []int64{20260710120000, 20260930120000} {
		require.NoError(t, db.QueryRowContext(ctx, `
SELECT count(*) FROM goadmin_uploads_goose_db_version WHERE version_id=$1 AND is_applied`, version).Scan(&count))
		require.Equal(t, 1, count)
	}
}

func assertUploadLifecycleRows(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	var status, hash string
	var fileType, count int
	require.NoError(t, db.QueryRowContext(ctx, `
SELECT status FROM upload_sessions WHERE object_path='preserved/session'`).Scan(&status))
	require.Equal(t, "cleaning", status)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT binding_hash FROM upload_finalizations`).Scan(&hash))
	require.Equal(t, strings.Repeat("a", 64), hash)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM file_deletions WHERE completed_at IS NULL`).Scan(&count))
	require.Equal(t, 1, count)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT file_type FROM files WHERE slug='preserved-audio'`).Scan(&fileType))
	require.Equal(t, 7, fileType)
}
