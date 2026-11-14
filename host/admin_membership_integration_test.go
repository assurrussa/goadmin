//go:build integration

package host //nolint:testpackage // verifies canonical auth transaction and admin projection

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/internal/identity"
)

func TestRequireActiveAdminUsesCanonicalTransaction(t *testing.T) {
	cfg, deps := assemblyFixture(t)
	runtime, err := New(t.Context(), cfg, deps)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, runtime.Close()) })
	auth := runtime.Auth.Runtime()
	provision := func(email string, member bool) goauth.SubjectID {
		account, e := auth.ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{Email: email, Password: assemblyPassword})
		require.NoError(t, e)
		if member {
			_, e = runtime.Auth.admins.ProvisionAccount(t.Context(), account, identity.NewUserID())
			require.NoError(t, e)
		}
		return account.Subject.ID
	}
	active := provision("membership-active@example.test", true)
	customer := provision("membership-customer@example.test", false)
	revoked := provision("membership-revoked@example.test", true)
	suspended := provision("membership-suspended@example.test", true)
	retired := provision("membership-retired@example.test", true)
	db := auth.Database()
	_, err = db.ExecContext(t.Context(), `UPDATE administrations SET deleted_at=now() WHERE subject_id=$1`, revoked)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `UPDATE auth_subjects SET status='suspended' WHERE id=$1`, suspended)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `UPDATE auth_subjects SET status='disabled', retired_at=now() WHERE id=$1`, retired)
	require.NoError(t, err)
	for _, subject := range []goauth.SubjectID{customer, revoked, suspended, retired, goauth.SubjectID(uuid.New()), {}} {
		require.ErrorIs(t, runtime.Auth.RequireActiveAdmin(t.Context(), subject), ErrAdminMembershipUnavailable)
	}
	// Cancellation is an infrastructure failure, never a membership denial.
	cancelled, stop := context.WithCancel(t.Context())
	stop()
	err = runtime.Auth.RequireActiveAdmin(cancelled, active)
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, ErrAdminMembershipUnavailable)
	// One connection proves the guard joins rather than opening a second query
	// connection while the auth transaction owns the only available one.
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, auth.InAuthTransaction(ctx, func(txctx context.Context) error {
		return runtime.Auth.RequireActiveAdmin(txctx, active)
	}))
	db.SetMaxOpenConns(0)
	for _, query := range []string{
		`UPDATE administrations SET deleted_at=now() WHERE subject_id=$1`,
		`UPDATE auth_subjects SET status='suspended' WHERE id=$1`,
	} {
		require.NoError(t, auth.InAuthTransaction(t.Context(), func(txctx context.Context) error {
			if e := runtime.Auth.RequireActiveAdmin(txctx, active); e != nil {
				return e
			}
			blocked, stop := context.WithTimeout(txctx, 3*time.Second)
			defer stop()
			mutation, e := db.BeginTx(blocked, nil)
			if e != nil {
				return e
			}
			defer func() { _ = mutation.Rollback() }()
			if _, e = mutation.ExecContext(blocked, `SET LOCAL lock_timeout='100ms'`); e != nil {
				return e
			}
			_, e = mutation.ExecContext(blocked, query, active)
			var pgerr *pgconn.PgError
			if !errors.As(e, &pgerr) || pgerr.Code != "55P03" {
				return errors.New("expected PostgreSQL lock timeout")
			}
			return nil
		}))
	}
	// Another adapter owns a different canonical auth handle even over the same pool.
	cfg.Auth.NotificationDelivery = goauth.NotificationDeliveryDisabled
	other, err := NewAuthAdapter(AuthAdapterConfig{
		Database: deps.Database, TxManager: NewTxManager(deps.Database), Runtime: cfg.Auth,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, other.Close()) })
	require.NoError(t, auth.InAuthTransaction(t.Context(), func(txctx context.Context) error {
		if e := other.RequireActiveAdmin(txctx, active); e == nil {
			return errors.New("foreign transaction context accepted")
		}
		return nil
	}))
	// Locks release at commit, so both previously blocked mutations can succeed.
	_, err = db.ExecContext(t.Context(), `UPDATE auth_subjects SET status='suspended' WHERE id=$1`, active)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `UPDATE auth_subjects SET status='active' WHERE id=$1`, active)
	require.NoError(t, err)
	// A committed revocation becomes immediately visible to the next policy call.
	_, err = db.ExecContext(t.Context(), `UPDATE administrations SET deleted_at=now() WHERE subject_id=$1`, active)
	require.NoError(t, err)
	require.ErrorIs(t, runtime.Auth.RequireActiveAdmin(t.Context(), active), ErrAdminMembershipUnavailable)
}
