//go:build integration

package userrepo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/stretchr/testify/require"

	adminhost "github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	userrepo "github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/userrepo"
	"github.com/assurrussa/goadmin/internal/admintx"
	identity "github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/tests"
)

func TestUserProjectionReadsCanonicalV02Account(t *testing.T) {
	ctx := context.Background()
	db, _, cleanup := tests.PrepareDB(ctx, t, "UserRepoV02")
	tx := adminhost.NewTxManager(db)
	auth, err := adminhost.NewAuthAdapter(adminhost.AuthAdapterConfig{
		Database: db, TxManager: tx, Runtime: tests.AuthRuntimeConfig(t),
		NotificationSender: tests.AuthNotificationSender(),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, auth.Close())
		cleanup(ctx)
	})
	repo := userrepo.Must(userrepo.NewOptions(db, tx))

	account, err := auth.Runtime().ProvisionTrustedLocalAccount(ctx, goauth.RegisterRequest{
		Email: "Canonical.User@Example.Test", Password: "Q7!user-repository-2026",
		Profile: goauth.BasicProfile{
			Username: "canonical-user", DisplayName: "Canonical User",
			GivenName: "Canonical", FamilyName: "User",
		},
	})
	require.NoError(t, err)
	publicID := identity.NewUserID()
	now := time.Now().UTC()
	insert := outbox.BuilderDollar().Insert("users").
		Columns("subject_id", "uuid", "bio", "created_at", "updated_at").
		Values(account.Subject.ID, publicID, "host bio", now, now)
	_, err = db.DB().Execx(ctx, "userrepo.insert_projection", insert)
	require.NoError(t, err)

	profile, err := repo.GetByUUID(ctx, publicID)
	require.NoError(t, err)
	require.Equal(t, account.Subject.ID, profile.SubjectID)
	require.Equal(t, "Canonical.User@Example.Test", profile.Email)
	require.Equal(t, "canonical-user", value(profile.Username))
	require.Equal(t, "Canonical", profile.Name)
	require.Equal(t, "User", value(profile.LastName))
	require.True(t, profile.IsEmailConfirmed())
	require.Equal(t, "host bio", value(profile.Bio))

	list, total, err := repo.GetList(ctx, datagrid.Filters{
		Page: 1, Limit: 10, SortBy: "email", SortOrder: "asc",
		Fields: map[string]any{"emailStatus": "confirmed"},
	})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, list, 1)

	profile.Bio = stringPtr("updated host bio")
	require.NoError(t, repo.Update(ctx, profile.ID, profile))
	updated, err := repo.GetByID(ctx, profile.ID)
	require.NoError(t, err)
	require.Equal(t, "updated host bio", value(updated.Bio))
	require.Equal(t, "Canonical", updated.Name)

	require.NoError(t, repo.SoftDeleteByID(ctx, profile.ID))
	deleted, err := repo.GetByID(ctx, profile.ID)
	require.NoError(t, err)
	require.True(t, deleted.DeletedAt.Valid)
	require.NoError(t, repo.RestoreByID(ctx, profile.ID))
	restored, err := repo.GetBySubjectID(ctx, account.Subject.ID)
	require.NoError(t, err)
	require.False(t, restored.DeletedAt.Valid)
}

func value(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func stringPtr(value string) *string { return &value }

// This requires the real PostgreSQL release fixture. The unit executor test is
// deliberately not a substitute for rollback across canonical and projection SQL.
func TestUserProjectionJoinsCanonicalCommandRollback(t *testing.T) {
	ctx := t.Context()
	db, _, cleanup := tests.PrepareDB(ctx, t, "UserCommandRollback")
	tx := adminhost.NewTxManager(db)
	auth, err := adminhost.NewAuthAdapter(adminhost.AuthAdapterConfig{
		Database: db, TxManager: tx, Runtime: tests.AuthRuntimeConfig(t), NotificationSender: tests.AuthNotificationSender(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, auth.Close()); cleanup(context.Background()) })
	repo := userrepo.Must(userrepo.NewOptions(db, tx))
	account, err := auth.Runtime().ProvisionTrustedLocalAccount(ctx, goauth.RegisterRequest{
		Email: "user-command-rollback@example.test", Password: "Unique-User-Rollback-2026!",
		Profile: goauth.BasicProfile{GivenName: "Original"},
	})
	require.NoError(t, err)
	publicID := identity.NewUserID()
	now := time.Now().UTC()
	_, err = db.DB().Execx(ctx, "user.insert", outbox.BuilderDollar().Insert("users").
		Columns("subject_id", "uuid", "bio", "created_at", "updated_at").Values(account.Subject.ID, publicID, "original bio", now, now))
	require.NoError(t, err)
	before, err := repo.GetByUUID(ctx, publicID)
	require.NoError(t, err)
	failure := errors.New("force rollback after email enqueue")
	err = auth.Runtime().InAuthTransaction(ctx, func(txCtx context.Context) error {
		executor, err := auth.Runtime().SQLExecutor(txCtx)
		if err != nil {
			return err
		}
		txCtx = admintx.WithExecutor(txCtx, executor, db.DB().Pool())
		current, err := repo.GetByID(txCtx, before.ID)
		if err != nil {
			return err
		}
		if _, err = auth.Runtime().UpdateBasicProfile(txCtx, account.Subject.ID, goauth.BasicProfile{GivenName: "Changed"}); err != nil {
			return err
		}
		current.Bio = stringPtr("changed bio")
		if err = repo.Update(txCtx, current.ID, current); err != nil {
			return err
		}
		if err = auth.Runtime().RequestEmailChange(txCtx, account.Subject.ID, "user-command-target@example.test"); err != nil {
			return err
		}
		return failure
	})
	require.ErrorIs(t, err, failure)
	after, err := repo.GetByID(ctx, before.ID)
	require.NoError(t, err)
	require.Equal(t, before.Version, after.Version)
	require.Equal(t, "original bio", value(after.Bio))
	current, err := auth.Runtime().GetAccount(ctx, account.Subject.ID)
	require.NoError(t, err)
	require.Equal(t, "Original", current.Profile.GivenName)
	var changes int
	err = auth.Runtime().Database().QueryRowContext(ctx, "SELECT count(*) FROM auth_email_change_records WHERE subject_id=$1", account.Subject.ID).Scan(&changes)
	require.NoError(t, err)
	require.Zero(t, changes)
}
