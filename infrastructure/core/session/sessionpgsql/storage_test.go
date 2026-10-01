package sessionpgsql_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/infrastructure/core/session/sessionpgsql"
)

func TestStorageCancellationAndEmptyInputs(t *testing.T) {
	_, err := sessionpgsql.New(nil)
	require.ErrorContains(t, err, "database is required")
	db, err := sql.Open("pgx", "postgres://unused:unused@localhost/unused")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	store, err := sessionpgsql.New(db)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	value, err := store.GetWithContext(ctx, "")
	require.NoError(t, err)
	require.Nil(t, value)
	require.NoError(t, store.SetWithContext(ctx, "", []byte("value"), time.Minute))
	require.NoError(t, store.SetWithContext(ctx, "key", nil, time.Minute))
	require.NoError(t, store.DeleteWithContext(ctx, ""))
	_, err = store.GetWithContext(ctx, "key")
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, store.SetWithContext(ctx, "key", []byte("value"), time.Minute), context.Canceled)
	require.ErrorIs(t, store.DeleteWithContext(ctx, "key"), context.Canceled)
	require.ErrorIs(t, store.ResetWithContext(ctx), context.Canceled)
	require.ErrorIs(t, store.Cleanup(ctx), context.Canceled)
	require.NoError(t, store.Close())
	// Close must leave the borrowed DB open; closed SQL returns ErrDBClosed.
	require.ErrorIs(t, db.PingContext(ctx), context.Canceled)
	require.Zero(t, db.Stats().OpenConnections)
}
