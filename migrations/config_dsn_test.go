package migrations //nolint:testpackage // verifies the owned versus borrowed database boundary.

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/infrastructure/outbox"
)

func TestMigrateRejectsUnsupportedDSNBeforeOpeningDatabase(t *testing.T) {
	t.Parallel()

	for _, dsn := range []string{
		"postgres://admin:synthetic_secret@localhost/db?search_path=private_secret",
		"host=localhost password=synthetic_secret options='-c search_path=private_secret'",
		"host=localhost password=synthetic_secret role=private_secret",
	} {
		err := Migrate(t.Context(), DatabaseConfig{DSN: dsn}, nil)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "synthetic_secret")
		require.NotContains(t, err.Error(), "private_secret")
		require.NotContains(t, err.Error(), "ping goadmin migration database")
	}
}

func TestWithDatabaseLeavesBorrowedConnectionAndConfigAlone(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("pgx", "postgres://localhost/unused")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	sentinel := errors.New("callback sentinel")
	called := false
	err = withDatabase(t.Context(), DatabaseConfig{DSN: "invalid=private_secret options=private_secret"}, db,
		func(got *sql.DB, cfg outbox.StoragePgsqlConfig) error {
			called = true
			require.Same(t, db, got)
			require.Equal(t, outbox.StoragePgsqlConfig{}, cfg)
			return sentinel
		})
	require.ErrorIs(t, err, sentinel)
	require.True(t, called)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, db.PingContext(ctx), context.Canceled, "borrowed database must remain open")
}
