//go:build integration

package adminrepo_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/rbac"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	adminhost "github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	adminrepo "github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminrepo"
	identity "github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
	"github.com/assurrussa/goadmin/tests"
)

const integrationPassword = "Q7!admin-repository-2026"

type fixture struct {
	db      adminhost.StoragePgsqlClient
	repo    *adminrepo.Repo
	auth    *adminhost.AuthAdapter
	cleanup func(context.Context)
}

func newFixture(t *testing.T) (context.Context, *fixture) {
	t.Helper()
	ctx := context.Background()
	db, _, cleanup := tests.PrepareDB(ctx, t, "AdminRepoV02")
	tx := adminhost.NewTxManager(db)
	repo := adminrepo.Must(adminrepo.NewOptions(db, tx))
	auth, err := adminhost.NewAuthAdapter(adminhost.AuthAdapterConfig{
		Database: db, TxManager: tx, Runtime: tests.AuthRuntimeConfig(t),
		NotificationSender: tests.AuthNotificationSender(),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, auth.Close())
		cleanup(ctx)
	})

	return ctx, &fixture{db: db, repo: repo, auth: auth, cleanup: cleanup}
}

func (f *fixture) provision(t *testing.T, ctx context.Context, email, username string) (goauth.Account, models.Admin) {
	t.Helper()
	account, err := f.auth.Runtime().ProvisionTrustedLocalAccount(ctx, goauth.RegisterRequest{
		Email: email, Password: integrationPassword,
		Profile: goauth.BasicProfile{
			Username: username, DisplayName: username, GivenName: username, FamilyName: "Admin",
		},
	})
	require.NoError(t, err)
	admin, err := f.repo.ProvisionAccount(ctx, account, identity.NewUserID())
	require.NoError(t, err)

	return account, admin
}

func TestProjectionReadsCanonicalV02Identity(t *testing.T) {
	ctx, f := newFixture(t)
	account, created := f.provision(t, ctx, "Canonical.Admin@Example.Test", "canonical")

	byID, err := f.repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, account.Subject.ID, byID.SubjectID)
	require.Equal(t, "Canonical.Admin@Example.Test", byID.Email)
	require.Equal(t, "canonical", byID.Username)
	require.Equal(t, "canonical", byID.Name)
	require.Equal(t, "Admin", byID.LastName)
	require.NotNil(t, byID.GetEmailConfirmedAt())

	byUUID, err := f.repo.GetByUUID(ctx, created.UUID)
	require.NoError(t, err)
	require.Equal(t, created.ID, byUUID.ID)
	bySubject, err := f.repo.GetBySubjectID(ctx, account.Subject.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, bySubject.ID)

	list, total, err := f.repo.GetList(ctx, datagrid.Filters{
		Page: 1, Limit: 10, SortBy: "username", SortOrder: "asc",
		Search: "canonical.admin", Fields: map[string]any{"emailStatus": "confirmed"},
	})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, list, 1)
}

func TestListRoleNamesByAdminIDsUsesCanonicalAssignments(t *testing.T) {
	ctx, f := newFixture(t)
	account, first := f.provision(t, ctx, "roles-first@example.test", "roles-first")
	_, second := f.provision(t, ctx, "roles-second@example.test", "roles-second")
	roles, err := f.auth.Runtime().RBAC(nil)
	require.NoError(t, err)
	for _, role := range []rbac.Role{
		{Slug: "editor", Name: "Alpha Editor"},
		{Slug: "system", Name: "Zulu System", System: true},
	} {
		_, err := roles.UpsertRole(ctx, role)
		require.NoError(t, err)
		require.NoError(t, roles.AssignRole(ctx, account.Subject.ID, role.Slug))
	}

	got, err := f.repo.ListRoleNamesByAdminIDs(ctx, []int64{first.ID, second.ID})
	require.NoError(t, err)
	require.Equal(t, []string{"Zulu System", "Alpha Editor"}, got[first.ID])
	require.NotContains(t, got, second.ID)
}

func TestProjectionWritesCannotMutateCanonicalProfile(t *testing.T) {
	ctx, f := newFixture(t)
	_, created := f.provision(t, ctx, "immutable@example.test", "before")

	created.Name = "must-not-replace-canonical-name"
	created.Data = &models.AdminData{LastLoginAt: timePtr(time.Now().UTC())}
	require.NoError(t, f.repo.UpdateAdmin(ctx, created.ID, created))

	loaded, err := f.repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, "before", loaded.Name)
	require.NotNil(t, loaded.GetLastLoginAt())
	stale := created
	stale.Data = &models.AdminData{}
	require.ErrorIs(t, f.repo.UpdateAdmin(ctx, stale.ID, stale), adminrepo.ErrAdminVersionConflict)
	stillCurrent, err := f.repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.NotNil(t, stillCurrent.GetLastLoginAt())
}

func TestMembershipProvisionIsConcurrentAndForeignKeyBound(t *testing.T) {
	ctx, f := newFixture(t)
	account, err := f.auth.Runtime().ProvisionTrustedLocalAccount(ctx, goauth.RegisterRequest{
		Email: "concurrent@example.test", Password: integrationPassword,
	})
	require.NoError(t, err)
	publicID := identity.NewUserID()

	const workers = 16
	ids := make(chan int64, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, provisionErr := f.repo.ProvisionAdminMembership(ctx, account, publicID)
			ids <- id
			errs <- provisionErr
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	var first int64
	for err := range errs {
		require.NoError(t, err)
	}
	for id := range ids {
		if first == 0 {
			first = id
		}
		require.Equal(t, first, id)
	}

	unknown := goauth.Account{Subject: goauth.Subject{
		ID: goauth.NewSubjectID(), Status: goauth.SubjectStatusActive, SecurityVersion: 1,
	}}
	_, err = f.repo.ProvisionAdminMembership(ctx, unknown, identity.NewUserID())
	require.Error(t, err)
}

func TestSoftDeleteClosesAdminMembershipOnly(t *testing.T) {
	ctx, f := newFixture(t)
	account, admin := f.provision(t, ctx, "disabled-membership@example.test", "member")

	allowed, err := f.repo.HasAdminMembership(ctx, account.Subject.ID)
	require.NoError(t, err)
	require.True(t, allowed)
	require.NoError(t, f.repo.SoftDeleteByID(ctx, admin.ID))
	allowed, err = f.repo.HasAdminMembership(ctx, account.Subject.ID)
	require.NoError(t, err)
	require.False(t, allowed)
	bySubject, err := f.repo.GetBySubjectID(ctx, account.Subject.ID)
	require.NoError(t, err)
	require.Zero(t, bySubject.ID)

	canonical, err := f.auth.Runtime().GetAccount(ctx, account.Subject.ID)
	require.NoError(t, err)
	require.Equal(t, goauth.SubjectStatusActive, canonical.Subject.Status)

	_, err = f.repo.ProvisionAccount(ctx, account, identity.NewUserID())
	require.ErrorIs(t, err, goauth.ErrMembershipDenied)
	allowed, err = f.repo.HasAdminMembership(ctx, account.Subject.ID)
	require.NoError(t, err)
	require.False(t, allowed)
}

func TestSoftDeleteRejectsSuperAdminMembership(t *testing.T) {
	ctx, f := newFixture(t)
	account, admin := f.provision(t, ctx, "protected-super@example.test", "protected-super")
	_, err := f.db.DB().Exec(ctx, "test.adminrepo.super_role",
		"INSERT INTO auth_roles(public_id, slug, name) VALUES ($1, 'super_admin', 'Super Admin')",
		identity.NewUserID())
	require.NoError(t, err)
	_, err = f.db.DB().Exec(ctx, "test.adminrepo.super_assignment",
		"INSERT INTO auth_subject_roles(subject_id, role_id) SELECT $1, id FROM auth_roles WHERE slug = 'super_admin'",
		account.Subject.ID)
	require.NoError(t, err)
	require.ErrorIs(t, f.repo.SoftDeleteByID(ctx, admin.ID), pgx.ErrNoRows)
	current, err := f.repo.GetBySubjectID(ctx, account.Subject.ID)
	require.NoError(t, err)
	require.Equal(t, admin.ID, current.ID)
}

func timePtr(value time.Time) *time.Time { return &value }
