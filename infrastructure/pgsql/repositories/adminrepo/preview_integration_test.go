//go:build integration

package adminrepo_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	adminrepo "github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminrepo"
)

func TestUpdatePreviewRejectsStaleDetachAfterAttach(t *testing.T) {
	ctx, f := newFixture(t)
	_, admin := f.provision(t, ctx, "preview-cas@example.test", "preview-cas")
	const oldPreviewID int64 = 123
	const newPreviewID int64 = 456

	require.NoError(t, f.repo.UpdatePreview(ctx, admin, oldPreviewID))
	require.Zero(t, admin.GetPreviewFileID(), "repository update must not mutate the caller's snapshot")
	stale, err := f.repo.GetByID(ctx, admin.ID)
	require.NoError(t, err)
	require.Equal(t, oldPreviewID, stale.GetPreviewFileID())

	require.NoError(t, f.repo.UpdatePreview(ctx, stale, newPreviewID))
	require.ErrorIs(t, f.repo.UpdatePreview(ctx, stale, 0), adminrepo.ErrAdminVersionConflict)

	current, err := f.repo.GetByID(ctx, admin.ID)
	require.NoError(t, err)
	require.Equal(t, newPreviewID, current.GetPreviewFileID())
	require.NoError(t, f.repo.UpdatePreview(ctx, current, 0))

	cleared, err := f.repo.GetByID(ctx, admin.ID)
	require.NoError(t, err)
	require.Zero(t, cleared.GetPreviewFileID())
}
