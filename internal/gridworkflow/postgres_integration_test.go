//go:build integration

package gridworkflow //nolint:testpackage // Reuses internal preference schema fixtures.

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/tests"
)

func TestPostgresPreferencesCASIsolationAndCascade(t *testing.T) {
	ctx := context.Background()
	db, _, cleanup := tests.PrepareDB(ctx, t, "GridWorkflowPreferences")
	t.Cleanup(func() { cleanup(ctx) })
	createAdmin := func() int64 {
		subject := uuid.New()
		_, err := db.DB().Exec(ctx, "test.grid_preferences.subject",
			`INSERT INTO auth_subjects(id,status,created_at,updated_at) VALUES($1,'active',NOW(),NOW())`, subject)
		require.NoError(t, err)
		var id int64
		require.NoError(t, db.DB().ScanOne(ctx, "test.grid_preferences.admin", &id,
			`INSERT INTO administrations(subject_id,uuid) VALUES($1,$2) RETURNING id`, subject, uuid.New()))
		return id
	}
	admin1, admin2 := createAdmin(), createAdmin()
	store := NewPostgresStore(db.DB())
	payload := newSchema(testConfig()).defaultsPayload()
	_, version, err := store.Load(ctx, admin1, "queues")
	require.NoError(t, err)
	require.Zero(t, version)
	version, err = store.Save(ctx, admin1, "queues", 0, payload)
	require.NoError(t, err)
	require.Equal(t, int64(1), version)
	_, err = store.Save(ctx, admin1, "queues", 0, payload)
	require.ErrorIs(t, err, ErrConflict)
	_, version, err = store.Load(ctx, admin2, "queues")
	require.NoError(t, err)
	require.Zero(t, version)
	_, version, err = store.Load(ctx, admin1, "users")
	require.NoError(t, err)
	require.Zero(t, version)
	// Both concurrent saves compare the same revision; exactly one may succeed.
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() { _, e := store.Save(ctx, admin1, "queues", 1, payload); results <- e })
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, ErrConflict)
			conflicts++
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)
	got, version, err := store.Load(ctx, admin1, "queues")
	require.NoError(t, err)
	require.Equal(t, int64(2), version)
	require.Equal(t, payload, got)
	_, err = db.DB().Exec(ctx, "test.grid_preferences.delete", `DELETE FROM administrations WHERE id=$1`, admin1)
	require.NoError(t, err)
	_, version, err = store.Load(ctx, admin1, "queues")
	require.NoError(t, err)
	require.Zero(t, version)
	_, err = store.Save(ctx, admin1, "queues", 0, payload)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrConflict)
}
