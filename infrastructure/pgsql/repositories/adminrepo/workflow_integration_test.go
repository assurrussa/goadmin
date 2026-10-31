//go:build integration

package adminrepo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/stretchr/testify/require"

	adminhost "github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/userrepo"
	"github.com/assurrussa/goadmin/internal/admintx"
	"github.com/assurrussa/goadmin/internal/identity"
)

func workflowUser(t *testing.T, f *fixture, email string) (goauth.Account, int64) {
	t.Helper()
	account, err := f.auth.Runtime().ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{Email: email, Password: integrationPassword})
	require.NoError(t, err)
	var id int64
	require.NoError(t, f.db.DB().ScanOne(t.Context(), "test.workflow.user", &id,
		`INSERT INTO users(subject_id,uuid,created_at,updated_at) VALUES($1,$2,NOW(),NOW()) RETURNING id`, account.Subject.ID, identity.NewUserID()))
	return account, id
}

func TestWorkflowStatusAndProjectionRollbackTogether(t *testing.T) {
	_, f := newFixture(t)
	account, id := workflowUser(t, f, "workflow-rollback@example.test")
	repo := userrepo.Must(userrepo.NewOptions(f.db, adminhost.NewTxManager(f.db)))
	rollback := errors.New("reject projection command")
	err := f.auth.Runtime().InAuthTransaction(t.Context(), func(ctx context.Context) error {
		executor, err := f.auth.Runtime().SQLExecutor(ctx)
		if err != nil {
			return err
		}
		ctx = admintx.WithExecutor(ctx, executor, f.db.DB().Pool())
		if _, err = f.auth.Runtime().SetSubjectStatus(ctx, account.Subject.ID, goauth.SubjectStatusDisabled); err != nil {
			return err
		}
		if err = repo.SoftDeleteByID(ctx, id); err != nil {
			return err
		}
		return rollback
	})
	require.ErrorIs(t, err, rollback)
	current, err := f.auth.Runtime().GetAccount(t.Context(), account.Subject.ID)
	require.NoError(t, err)
	require.Equal(t, goauth.SubjectStatusActive, current.Subject.Status)
	projection, err := repo.GetByID(t.Context(), id)
	require.NoError(t, err)
	require.False(t, projection.DeletedAt.Valid)
}

func TestWorkflowDeleteSerializesWithAdminProvisioning(t *testing.T) {
	_, f := newFixture(t)
	account, _ := workflowUser(t, f, "workflow-race@example.test")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ready := make(chan struct{})
	done := make(chan error, 1)
	err := f.auth.Runtime().InAuthTransaction(ctx, func(txCtx context.Context) error {
		executor, err := f.auth.Runtime().SQLExecutor(txCtx)
		if err != nil {
			return err
		}
		txCtx = admintx.WithExecutor(txCtx, executor, f.db.DB().Pool())
		var status string
		if err = admintx.Wrap(f.db.DB()).ScanOne(txCtx, "test.workflow.lock", &status, `SELECT status FROM auth_subjects WHERE id=$1 FOR UPDATE`, account.Subject.ID); err != nil {
			return err
		}
		member, err := f.repo.HasAdminMembership(txCtx, account.Subject.ID)
		if err != nil {
			return err
		}
		require.False(t, member)
		go func() { close(ready); _, e := f.repo.ProvisionAccount(ctx, account, identity.NewUserID()); done <- e }()
		<-ready
		_, err = f.auth.Runtime().SetSubjectStatus(txCtx, account.Subject.ID, goauth.SubjectStatusDisabled)
		return err
	})
	require.NoError(t, err)
	select {
	case err = <-done:
		require.ErrorIs(t, err, goauth.ErrMembershipDenied)
	case <-ctx.Done():
		t.Fatal("provisioning did not finish after subject lock release")
	}
	member, err := f.repo.HasAdminMembership(ctx, account.Subject.ID)
	require.NoError(t, err)
	require.False(t, member)
}
