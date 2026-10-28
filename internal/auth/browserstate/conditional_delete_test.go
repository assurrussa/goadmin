package browserstate_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/internal/auth/browserstate"
)

func TestConditionalCleanupPreservesNewVersionAndOtherOwner(t *testing.T) {
	store := browserstate.NewMemory()
	ctx := t.Context()
	require.NoError(t, store.Create(ctx, "browser", browserstate.Record{}, time.Hour))
	old, found, err := store.Load(ctx, "browser")
	require.NoError(t, err)
	require.True(t, found)
	claimed, err := store.Claim(ctx, "browser", old.Version, "refresh-owner")
	require.NoError(t, err)
	require.True(t, claimed)
	deleted, err := store.DeleteIfCurrent(ctx, "browser", old.Version, "")
	require.NoError(t, err)
	require.False(t, deleted, "a passive reader cannot erase an active owner")
	completed, err := store.Complete(ctx, "browser", old.Version, "refresh-owner", old)
	require.NoError(t, err)
	require.True(t, completed)
	deleted, err = store.DeleteIfCurrent(ctx, "browser", old.Version, "refresh-owner")
	require.NoError(t, err)
	require.False(t, deleted, "N cleanup cannot erase N+1")
	current, found, err := store.Load(ctx, "browser")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, old.ExpiresAt, current.ExpiresAt, "CAS cannot renew the hard deadline")
	deleted, err = store.DeleteIfCurrent(ctx, "browser", current.Version, current.Owner)
	require.NoError(t, err)
	require.True(t, deleted)
}
