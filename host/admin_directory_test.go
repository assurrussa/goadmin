package host //nolint:testpackage // checks authorization before database use

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type directoryPermissions struct{ denied bool }

func (p directoryPermissions) CheckPermission(context.Context, string, PermissionKey) error {
	if p.denied {
		return ErrAdminDirectoryDenied
	}
	return nil
}

func TestAdminDirectoryRejectsInvalidDependencies(t *testing.T) {
	key := NewPermissionKey("directory", "read")
	_, err := NewAdminDirectory(nil, directoryPermissions{}, key)
	require.ErrorIs(t, err, ErrAdminDirectoryInvalid)
	_, err = NewAdminDirectory(new(sql.DB), nil, key)
	require.ErrorIs(t, err, ErrAdminDirectoryInvalid)
	_, err = NewAdminDirectory(new(sql.DB), directoryPermissions{}, PermissionKey{})
	require.ErrorIs(t, err, ErrAdminDirectoryInvalid)
}

func TestAdminDirectoryGuardsBeforeDatabase(t *testing.T) {
	key := NewPermissionKey("directory", "read")
	actor := Actor{AdminID: 1, SubjectID: uuid.NewString()}
	d, err := NewAdminDirectory(new(sql.DB), directoryPermissions{denied: true}, key)
	require.NoError(t, err)
	_, err = d.Search(t.Context(), actor, "query", 20)
	require.ErrorIs(t, err, ErrAdminDirectoryDenied)
	require.ErrorIs(t, d.LockActive(t.Context(), nil, actor.SubjectID), ErrAdminDirectoryInvalid)
	d.permissions = directoryPermissions{}
	_, err = d.Search(t.Context(), Actor{}, "query", 20)
	require.ErrorIs(t, err, ErrAdminDirectoryDenied)
	for _, q := range []string{"", "a", "line\nbreak", "\x00bad"} {
		_, err = d.Search(t.Context(), actor, q, 20)
		require.ErrorIs(t, err, ErrAdminDirectoryInvalid)
	}
	for _, limit := range []int{0, 51} {
		_, err = d.Search(t.Context(), actor, "query", limit)
		require.ErrorIs(t, err, ErrAdminDirectoryInvalid)
	}
	require.ErrorIs(t, d.LockActive(t.Context(), nil, actor.SubjectID), ErrAdminDirectoryInvalid)
}
