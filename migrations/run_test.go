package migrations //nolint:testpackage // required

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/assurrussa/goauth/postgres"
	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoreAdminProjectionMigrationDoesNotRequireLegacyAuthColumns(t *testing.T) {
	t.Parallel()

	sql, err := os.ReadFile("sql/20250524151219_create_administrations_table.sql")
	require.NoError(t, err)

	content := strings.ToLower(string(sql))
	for _, column := range []string{
		"email",
		"username",
		"name",
		"last_name",
		"password_hash",
		"pwd_version",
	} {
		require.NotContains(t, content, "\n    "+column+" ")
	}
}

func TestCoreMigrationTableNameIsStable(t *testing.T) {
	t.Parallel()

	require.Equal(t, "goadmin_goose_db_version", TableName)
}

func TestUploadsProviderUsesCanonicalMigrationsExceptCoreFiles(t *testing.T) {
	t.Parallel()

	require.Equal(t, "goadmin_uploads_goose_db_version", uploadsTableName)
	require.NotEqual(t, TableName, uploadsTableName)
	db, err := sql.Open("pgx", "postgres://localhost/unused")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	provider, err := uploadsProvider(db)
	require.NoError(t, err)
	canonical, err := uploadhost.MigrationFiles()
	require.NoError(t, err)
	var expected []string
	for _, name := range canonical {
		if name != uploadsFilesMigration {
			expected = append(expected, name)
		}
	}
	sources := provider.ListSources()
	actual := make([]string, 0, len(sources))
	for _, source := range sources {
		actual = append(actual, source.Path)
	}
	require.Equal(t, []string{
		"20260710120000_create_upload_sessions.sql",
		"20260930120000_upload_lifecycle_safety.sql",
	}, actual)
	require.Equal(t, expected, actual)
}

func TestFirstAdminSetupTokenMigrationStoresOnlyFixedLengthHash(t *testing.T) {
	t.Parallel()

	sql, err := os.ReadFile("sql/20260710120000_create_first_admin_setup_tokens.sql")
	require.NoError(t, err)

	content := strings.ToLower(string(sql))
	require.Contains(t, content, "token_hash  bytea")
	require.Contains(t, content, "octet_length(token_hash) = 32")
	require.NotContains(t, content, "raw_token")
	require.NotContains(t, content, "token_value")
}

func TestOutboxV012MigrationPacksStayIdenticalAndIdempotent(t *testing.T) {
	t.Parallel()

	const migration = "20260829120000_upgrade_outbox_v012_schema.sql"
	coreSQL, err := os.ReadFile("sql/" + migration)
	require.NoError(t, err)
	monolithSQL, err := os.ReadFile("../db/migrations/" + migration)
	require.NoError(t, err)
	require.Equal(t, coreSQL, monolithSQL)

	content := strings.ToLower(string(coreSQL))
	for _, clause := range []string{
		"add column if not exists schema_version integer not null default 1",
		"add column if not exists lease_token uuid not null default",
		"add column if not exists deduplication_key text null",
		"create index if not exists jobs_capability_claim_index",
		"create unique index if not exists jobs_deduplication_key_unique",
		"create table if not exists outbox_job_idempotency_keys",
		"create index if not exists outbox_job_idempotency_keys_created_at_index",
	} {
		require.Contains(t, content, clause)
	}
}

func TestCoreMigrationPublicConfigSurfaceCompiles(t *testing.T) {
	t.Parallel()

	_ = DatabaseConfig{}
	requireMigrateSignature(Migrate)
	requireResetSignature(Reset)
}

func requireMigrateSignature(func(context.Context, DatabaseConfig, *sql.DB) error) {
}

func requireResetSignature(func(context.Context, DatabaseConfig, *sql.DB, postgres.ResetConfirmation) error) {
}

func TestCoreMigrationsAreV02OnlyAndNonDestructive(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir("sql")
	require.NoError(t, err)
	var combined strings.Builder
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		content, readErr := os.ReadFile("sql/" + entry.Name())
		require.NoError(t, readErr)
		_, _ = combined.Write(content)
	}
	content := strings.ToLower(combined.String())
	require.NotContains(t, content, "drop table if exists auth_")
	require.Contains(t, content, "references auth_subjects (id)")
	require.Contains(t, content, "create table if not exists users")
}

func TestCoreMigrationDatabaseConfigDSNNormalizesStorageConfig(t *testing.T) {
	t.Parallel()

	password := strings.Repeat("s", 6)
	cfg, err := storageConfig(DatabaseConfig{
		DSN:                 postgresDSN("goadmin", password, "pgsql.local:15432", "goadmindb", "require"),
		MinConnectionsCount: 2,
		MaxConnectionsCount: 7,
		MaxConnIdleTime:     2 * time.Minute,
		MaxConnLifeTime:     30 * time.Minute,
	})
	require.NoError(t, err)

	assert.Equal(t, "pgsql.local:15432", cfg.Address)
	assert.Equal(t, "goadmin", cfg.Username)
	assert.Equal(t, password, cfg.Password)
	assert.Equal(t, "goadmindb", cfg.Database)
	assert.Equal(t, "require", cfg.SSLMode)
	assert.Equal(t, int32(2), cfg.MinConnectionsCount)
	assert.Equal(t, int32(7), cfg.MaxConnectionsCount)
	assert.Equal(t, 2*time.Minute, cfg.MaxConnIdleTime)
	assert.Equal(t, 30*time.Minute, cfg.MaxConnLifeTime)
}

func postgresDSN(username string, password string, host string, database string, sslMode string) string {
	parsed := url.URL{
		Scheme: "postgresql",
		User:   url.UserPassword(username, password),
		Host:   host,
		Path:   database,
	}
	query := parsed.Query()
	query.Set("sslmode", sslMode)
	parsed.RawQuery = query.Encode()

	return parsed.String()
}
