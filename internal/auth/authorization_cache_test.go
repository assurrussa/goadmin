package auth //nolint:testpackage // exercises optional snapshot without a database-backed fallback

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/rbac"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/internal/admintx"
	"github.com/assurrussa/goadmin/internal/authsnapshot"
)

func TestSnapshotGuardWarmDecisionsNeedNoRepositories(t *testing.T) {
	id := goauth.NewSubjectID()
	cache, err := authsnapshot.New(t.Context(), func(context.Context) (authsnapshot.Data, error) {
		return authsnapshot.Data{Members: []goauth.SubjectID{id}, AdminIDs: map[int64]goauth.SubjectID{17: id}, RBAC: rbac.Snapshot{
			Roles: []rbac.Role{{ID: 1, Slug: "editor"}}, Permissions: []rbac.Permission{{ID: 2, Key: "pages:read"}},
			RolePermissions: []rbac.RolePermission{{RoleID: 1, PermissionID: 2}},
			SubjectRoles:    []rbac.SubjectRole{{SubjectID: id, RoleID: 1}},
		}}, nil
	}, authsnapshot.Config{RefreshInterval: time.Hour, MaxAge: 2 * time.Hour})
	require.NoError(t, err)
	defer cache.Close()
	// Deliberately no database repositories: any accidental fallback panics.
	guard := &GuardService{authorization: cache}
	key := NewPermissionKey("pages", "read")
	for range 1000 {
		require.True(t, guard.SubjectGuardCheck(t.Context(), id.String(), key))
		require.False(t, guard.SubjectGuardCheck(t.Context(), goauth.NewSubjectID().String(), key))
		roles, err := guard.GetRolesSubject(t.Context(), id.String())
		require.NoError(t, err)
		require.Len(t, roles, 1)
		perms, err := guard.GetPermissions(t.Context(), roles)
		require.NoError(t, err)
		require.Equal(t, key.Domain, perms[1][2].Domain)
		subject, active := guard.CachedAdminSubject(t.Context(), 17)
		require.True(t, active)
		require.Equal(t, id.String(), subject)
		subject, active = guard.CachedAdminSubject(t.Context(), 999)
		require.True(t, active)
		require.Empty(t, subject)
	}
	restricted := WithRolePreview(t.Context(), map[PermissionKey]bool{})
	require.False(t, guard.SubjectGuardCheck(restricted, id.String(), key))
	require.False(t, guard.useAuthorizationCache(admintx.WithExecutor(t.Context(), nil)), "mutation scope reads authoritative state")
	guard.transactionActive = func(context.Context) (bool, error) { return false, errors.New("foreign transaction") }
	require.False(t, guard.SubjectGuardCheck(t.Context(), id.String(), key))
	_, err = guard.GetRolesSubject(t.Context(), id.String())
	require.Error(t, err)
	guard.transactionActive = nil
	cache.Close()
	require.False(t, guard.SubjectGuardCheck(t.Context(), id.String(), key))
	_, err = guard.GetRolesSubject(t.Context(), id.String())
	require.Error(t, err)
}
