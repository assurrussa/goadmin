//go:build integration

package host //nolint:testpackage // verifies canonical transaction/cache wiring

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/rbac"
	"github.com/stretchr/testify/require"

	internalauth "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/identity"
)

func TestAuthorizationSnapshotCommitRollbackAndExternalRefresh(t *testing.T) {
	cfg, deps := assemblyFixture(t)
	cfg.Auth.NotificationDelivery = goauth.NotificationDeliveryDisabled
	newAdapter := func() *AuthAdapter {
		a, err := NewAuthAdapter(AuthAdapterConfig{
			Database: deps.Database, TxManager: NewTxManager(deps.Database), Runtime: cfg.Auth,
			AuthorizationCache: &AuthorizationCacheConfig{
				RefreshInterval: 100 * time.Millisecond, MaxAge: time.Second,
				LoadTimeout: 500 * time.Millisecond,
			},
		})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, a.Close()) })
		return a
	}
	a := newAdapter()
	checker, err := a.SubjectPermissionChecker(DiscardLogger())
	require.NoError(t, err)
	account, err := a.Runtime().ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
		Email: "snapshot-admin@example.test", Password: assemblyPassword,
	})
	require.NoError(t, err)
	admin, err := a.admins.ProvisionAccount(t.Context(), account, identity.NewUserID())
	require.NoError(t, err)
	seedRBAC, err := a.Runtime().RBAC(nil)
	require.NoError(t, err)
	_, err = seedRBAC.UpsertPermission(t.Context(), rbac.Permission{Key: "pages:read"})
	require.NoError(t, err)
	role, err := a.inner.Roles().CreateRole(t.Context(), internalauth.CreateRoleInput{
		Slug: "snapshot-editor", Name: "Snapshot editor",
		Permissions: []internalauth.PermissionKey{internalauth.NewPermissionKey("pages", "read")},
	})
	require.NoError(t, err)
	require.NoError(t, a.inner.Roles().AssignRolesToSubject(t.Context(), account.Subject.ID.String(), []int64{role.ID}))
	customer, err := a.Runtime().ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
		Email: "snapshot-customer@example.test", Password: assemblyPassword,
	})
	require.NoError(t, err)
	require.NoError(t, seedRBAC.ReplaceSubjectRoles(t.Context(), customer.Subject.ID, []int64{role.ID}))
	customerAllowed, err := seedRBAC.Check(t.Context(), customer.Subject.ID, "pages:read")
	require.NoError(t, err)
	require.True(t, customerAllowed, "admin cache must not override generic user RBAC")
	require.ErrorIs(t,
		checker.CheckPermission(t.Context(), customer.Subject.ID.String(), NewPermissionKey("pages", "read")),
		ErrSubjectPermissionDenied,
	)
	key := NewPermissionKey("pages", "read")
	allowed := func(c *SubjectPermissionChecker) bool {
		return c.CheckPermission(t.Context(), account.Subject.ID.String(), key) == nil
	}
	require.Eventually(t, func() bool { return allowed(checker) }, 3*time.Second, 10*time.Millisecond)
	// Another adapter demonstrates periodic propagation without a per-request query.
	b := newAdapter()
	other, err := b.SubjectPermissionChecker(DiscardLogger())
	require.NoError(t, err)
	require.True(t, allowed(other))
	login, err := a.Runtime().Login(t.Context(), goauth.LoginRequest{Realm: goauth.RealmAdmin, Credential: goauth.Credential{
		Identifier: goauth.IdentifierInput{Scheme: goauth.IdentifierSchemeEmail, Value: "snapshot-admin@example.test"},
		Password:   assemblyPassword,
	}})
	require.NoError(t, err)
	rollback := errors.New("rollback requested")

	// Direct Runtime scope (without GoAdmin context marker) must read uncommitted
	// canonical RBAC state, not a cached grant. Rollback keeps outside grants.
	err = a.Runtime().InAuthTransaction(t.Context(), func(ctx context.Context) error {
		if e := seedRBAC.ReplaceRolePermissions(ctx, role.ID, []rbac.PermissionKey{}); e != nil {
			return e
		}
		require.ErrorIs(t, checker.CheckPermission(ctx, account.Subject.ID.String(), key), ErrSubjectPermissionDenied)
		require.True(t, allowed(checker))
		return rollback
	})
	require.ErrorIs(t, err, rollback)
	require.True(t, allowed(checker))
	// A foreign legacy pgx transaction cannot cause a separate committed delete.
	err = NewTxManager(deps.Database).RunInTx(t.Context(), func(ctx context.Context) error {
		return a.admins.SoftDeleteByID(ctx, admin.ID)
	})
	require.ErrorContains(t, err, "canonical auth transaction")
	require.True(t, allowed(checker))
	err = a.Runtime().InAuthTransaction(t.Context(), func(ctx context.Context) error {
		if e := a.admins.SoftDeleteByID(ctx, admin.ID); e != nil {
			return e
		}
		require.True(t, allowed(checker), "nested SQL must not invalidate before outer commit")
		_, loginErr := a.Runtime().AuthenticateSession(ctx, login.Tokens.AccessToken)
		require.ErrorIs(t, loginErr, goauth.ErrMembershipDenied, "canonical introspection sees its own uncommitted membership deletion")
		return rollback
	})
	require.ErrorIs(t, err, rollback)
	require.True(t, allowed(checker))
	require.NoError(t, a.admins.SoftDeleteByID(t.Context(), admin.ID))
	require.False(t, allowed(checker), "commit fences cached membership before returning")
	require.Eventually(t, func() bool { return !allowed(other) }, 3*time.Second, 10*time.Millisecond)
	// External SQL has no local hook; the documented timer repairs both adapters.
	_, err = a.Runtime().Database().ExecContext(t.Context(), `UPDATE administrations SET deleted_at=NULL WHERE id=$1`, admin.ID)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return allowed(checker) && allowed(other) }, 3*time.Second, 10*time.Millisecond)
	roles, err := a.Runtime().RBAC(nil)
	require.NoError(t, err)
	require.NoError(t, roles.ReplaceRolePermissions(t.Context(), role.ID, []rbac.PermissionKey{}))
	require.Eventually(t, func() bool { return !allowed(checker) && !allowed(other) }, 3*time.Second, 10*time.Millisecond)
}
