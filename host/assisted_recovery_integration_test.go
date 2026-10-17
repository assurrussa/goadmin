//go:build integration

package host //nolint:testpackage // Verifies the private canonical policy on its exact managed handle.

import (
	"context"
	"testing"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/rbac"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/internal/identity"
)

func TestAssistedRecoveryDedicatedGrantAndSoleOwnerPolicy(t *testing.T) {
	cfg, deps := assemblyFixture(t)
	cfg.Auth.NotificationDelivery = goauth.NotificationDeliveryDisabled
	adapter, err := NewAuthAdapter(AuthAdapterConfig{
		Database: deps.Database, TxManager: NewTxManager(deps.Database), Runtime: cfg.Auth,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adapter.Close()) })
	actor, err := adapter.Runtime().ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
		Email: "recovery.policy.operator@example.test", Password: assemblyPassword,
	})
	require.NoError(t, err)
	target, err := adapter.Runtime().ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
		Email: "recovery.policy.target@example.test", Password: assemblyPassword,
	})
	require.NoError(t, err)
	_, err = adapter.admins.ProvisionAccount(t.Context(), actor, identity.NewUserID())
	require.NoError(t, err)
	policy, err := NewAssistedRecoveryAuthorization(adapter)
	require.NoError(t, err)
	roles, err := adapter.Runtime().RBAC(nil)
	require.NoError(t, err)
	role, err := roles.UpsertRole(t.Context(), rbac.Role{Slug: "recovery_policy", Name: "Recovery policy"})
	require.NoError(t, err)
	permission, err := roles.UpsertPermission(t.Context(), rbac.Permission{
		Key: "administrations:update", Description: "Ordinary update",
	})
	require.NoError(t, err)
	require.NoError(t, roles.ReplaceRolePermissions(t.Context(), role.ID, []rbac.PermissionKey{permission.Key}))
	require.NoError(t, roles.ReplaceSubjectRoles(t.Context(), actor.Subject.ID, []int64{role.ID}))
	auth := goauth.AuthContext{SubjectID: actor.Subject.ID, Realm: goauth.RealmAdmin}
	check := func(want error) {
		t.Helper()
		require.NoError(t, adapter.Runtime().InOwnedAuthTransaction(t.Context(), func(ctx context.Context) error {
			require.ErrorIs(t, policy.AuthorizeAssistedRecovery(ctx, auth, target.Subject.ID,
				"Verified person", "Known support case"), want)
			return nil
		}))
	}
	check(goauth.ErrAssistedRecoveryDenied)
	permission, err = roles.UpsertPermission(t.Context(), rbac.Permission{
		Key: "account_recovery:create", Description: "Account recovery",
	})
	require.NoError(t, err)
	require.NoError(t, roles.ReplaceRolePermissions(t.Context(), role.ID, []rbac.PermissionKey{permission.Key}))
	check(nil)
	super, err := roles.UpsertRole(t.Context(), rbac.Role{Slug: "super_admin", Name: "Owner", System: true})
	require.NoError(t, err)
	require.NoError(t, roles.ReplaceSubjectRoles(t.Context(), actor.Subject.ID, []int64{super.ID}))
	require.NoError(t, adapter.Runtime().InOwnedAuthTransaction(t.Context(), func(ctx context.Context) error {
		require.NoError(t, policy.AuthorizeOwnerAssistedRecovery(ctx, actor.Subject.ID))
		require.ErrorIs(t, policy.AuthorizeAssistedRecovery(ctx, auth, actor.Subject.ID,
			"Verified", "Known case"), goauth.ErrAssistedRecoveryDenied)
		return nil
	}))
	_, err = adapter.admins.ProvisionAccount(t.Context(), target, identity.NewUserID())
	require.NoError(t, err)
	require.NoError(t, roles.ReplaceSubjectRoles(t.Context(), target.Subject.ID, []int64{super.ID}))
	require.NoError(t, adapter.Runtime().InOwnedAuthTransaction(t.Context(), func(ctx context.Context) error {
		require.ErrorIs(t, policy.AuthorizeOwnerAssistedRecovery(ctx, actor.Subject.ID), goauth.ErrAssistedRecoveryDenied)
		return nil
	}))
	// A disabled second owner must not activate after the sole-active-owner check.
	_, err = adapter.Runtime().Database().ExecContext(t.Context(),
		`UPDATE auth_subjects SET status='disabled' WHERE id=$1`, target.Subject.ID)
	require.NoError(t, err)
	require.NoError(t, adapter.Runtime().InOwnedAuthTransaction(t.Context(), func(ctx context.Context) error {
		require.NoError(t, policy.AuthorizeOwnerAssistedRecovery(ctx, actor.Subject.ID))
		other, err := adapter.Runtime().Database().BeginTx(t.Context(), nil)
		require.NoError(t, err)
		defer func() { _ = other.Rollback() }()
		_, err = other.ExecContext(t.Context(), `SET LOCAL lock_timeout='100ms'`)
		require.NoError(t, err)
		_, err = other.ExecContext(t.Context(), `UPDATE auth_subjects SET status='active' WHERE id=$1`, target.Subject.ID)
		require.ErrorContains(t, err, "lock timeout")
		return nil
	}))
}
