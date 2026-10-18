//go:build integration

package admission_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/rbac"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/host"
	admission "github.com/assurrussa/goadmin/internal/admission"
	"github.com/assurrussa/goadmin/tests"
)

type proof struct {
	identity admission.Identity
	denied   bool
}

func (p *proof) Verify(context.Context, string) (admission.Identity, error) {
	if p.denied {
		return admission.Identity{}, admission.ErrDenied
	}
	return p.identity, nil
}

func (p *proof) Revalidate(context.Context, string, admission.Identity) error {
	if p.denied {
		return admission.ErrDenied
	}
	return nil
}

type pgFixture struct {
	auth         *host.AuthAdapter
	store        *admission.Postgres
	service      *admission.Service
	roles        *rbac.Service
	actor        goauth.Account
	role         rbac.Role
	operatorRole rbac.Role
	proof        *proof
	now          time.Time
}

func setup(t *testing.T) *pgFixture {
	t.Helper()
	ctx := t.Context()
	db, _, cleanup := tests.PrepareDB(ctx, t, "SSOAdmission")
	auth, err := host.NewAuthAdapter(host.AuthAdapterConfig{
		Database: db, TxManager: host.NewTxManager(db),
		Runtime: tests.AuthRuntimeConfig(t), NotificationSender: tests.AuthNotificationSender(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, auth.Close()); cleanup(context.Background()) })
	f := &pgFixture{auth: auth, now: time.Now().UTC()}
	f.actor, err = auth.Runtime().ProvisionTrustedLocalAccount(ctx, goauth.RegisterRequest{
		Email: "admission-operator@example.test", Password: "Synthetic-Admission-Operator-2026!",
	})
	require.NoError(t, err)
	_, err = auth.Runtime().Database().ExecContext(ctx, `INSERT INTO administrations(subject_id,uuid,version,data)
 VALUES($1,$2,0,'{}'::jsonb)`, f.actor.Subject.ID, uuid.NewString())
	require.NoError(t, err)
	f.roles, err = auth.Runtime().RBAC(nil)
	require.NoError(t, err)
	f.role, err = f.roles.CreateRole(ctx, rbac.Role{Slug: "admission-reader", Name: "Admission reader"}, nil)
	require.NoError(t, err)
	grant, err := admission.GrantPermission(f.role.PublicID)
	require.NoError(t, err)
	keys := []rbac.PermissionKey{
		rbac.MustPermissionKey("admission", "read"), rbac.MustPermissionKey("admission", "invite"),
		rbac.MustPermissionKey("admission", "approve"), rbac.MustPermissionKey("admission", "reject"),
		rbac.MustPermissionKey("roles", "assign"), grant,
	}
	for _, key := range keys {
		_, err = f.roles.UpsertPermission(ctx, rbac.Permission{Key: key})
		require.NoError(t, err)
	}
	f.operatorRole, err = f.roles.CreateRole(ctx, rbac.Role{Slug: "admission-manager", Name: "Admission manager"}, keys)
	require.NoError(t, err)
	require.NoError(t, f.roles.ReplaceSubjectRoles(ctx, f.actor.Subject.ID, []int64{f.operatorRole.ID}))
	scope := admission.Scope{Issuer: "https://issuer.example.test", Project: "project-one", Client: "client-one"}
	f.proof = &proof{identity: admission.Identity{
		Scope: scope, Subject: "alice", Email: "admission-alice@example.test",
		EmailVerified: true, ExpiresAt: f.now.Add(time.Hour),
	}}
	f.store, err = admission.NewPostgres(auth.Runtime(), func() time.Time { return f.now })
	require.NoError(t, err)
	f.service, err = admission.New(admission.Config{
		Mode: admission.ProjectPending, Scope: scope,
		Now: func() time.Time { return f.now }, NewID: uuid.NewString,
	}, f.store, f.proof)
	require.NoError(t, err)
	return f
}

func count(t *testing.T, f *pgFixture, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, f.auth.Runtime().Database().QueryRowContext(t.Context(), query, args...).Scan(&n))
	return n
}

func TestPostgresAdmissionPendingApprovalAndNoLocalPassword(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	r, err := f.service.Admit(ctx, "verified-rp", "")
	require.NoError(t, err)
	require.Equal(t, admission.Pending, r.Status)
	require.Zero(t, count(t, f, "SELECT count(*) FROM administrations WHERE subject_id=$1", r.LocalSubject))
	require.Zero(t, count(t, f, "SELECT count(*) FROM auth_local_credentials WHERE subject_id=$1", r.LocalSubject))
	require.NoError(t, f.service.Approve(ctx,
		f.actor.Subject.ID.String(), "alice", r.Version, []int64{f.role.ID}, f.now.Add(time.Hour)))
	require.Zero(t, count(t, f, "SELECT count(*) FROM administrations WHERE subject_id=$1", r.LocalSubject))
	f.proof.denied = true
	_, err = f.service.Admit(ctx, "denied-rp", "")
	require.ErrorIs(t, err, admission.ErrDenied)
	f.proof.denied = false
	r, err = f.service.Admit(ctx, "fresh-rp", "")
	require.NoError(t, err)
	require.Equal(t, admission.Active, r.Status)
	require.Equal(t, 1, count(t, f,
		"SELECT count(*) FROM administrations WHERE subject_id=$1 AND deleted_at IS NULL", r.LocalSubject))
	require.Equal(t, 1, count(t, f,
		"SELECT count(*) FROM auth_subject_roles WHERE subject_id=$1 AND role_id=$2", r.LocalSubject, f.role.ID))
	require.Zero(t, count(t, f, "SELECT count(*) FROM auth_sessions WHERE subject_id=$1", r.LocalSubject))
}

func TestPostgresAdmissionConcurrentCallbacksAreIdempotent(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	i, err := f.service.Invite(ctx, f.actor.Subject.ID.String(), "alice", []int64{f.role.ID}, f.now.Add(time.Hour))
	require.NoError(t, err)
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := f.service.Admit(ctx, "rp", i.ID); results <- err }()
	}
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	require.Equal(t, 1, count(t, f,
		"SELECT count(*) FROM auth_identity_links WHERE issuer=$1 AND external_subject='alice'", f.proof.identity.Scope.Issuer))
	require.Equal(t, 1, count(t, f, "SELECT count(*) FROM admin_admissions WHERE external_subject='alice' AND status='active'"))
}

func TestPostgresAdmissionEmailCollisionDoesNotLink(t *testing.T) {
	f := setup(t)
	_, err := f.auth.Runtime().ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
		Email: f.proof.identity.Email, Password: "Synthetic-Existing-Account-2026!",
	})
	require.NoError(t, err)
	_, err = f.service.Admit(t.Context(), "rp", "")
	require.ErrorIs(t, err, goauth.ErrExplicitIdentityLink)
	require.Zero(t, count(t, f, "SELECT count(*) FROM admin_admissions"))
	require.Zero(t, count(t, f, "SELECT count(*) FROM auth_identity_links WHERE external_subject='alice'"))
}

func TestPostgresAdmissionAuditFailureRollsBackAccountAndMembership(t *testing.T) {
	f := setup(t)
	_, err := f.auth.Runtime().Database().ExecContext(t.Context(), `CREATE FUNCTION fail_admission_test_audit()
 RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic audit failure'; END; $$;
 CREATE TRIGGER fail_admission_audit BEFORE INSERT ON admin_admission_events
 FOR EACH ROW EXECUTE FUNCTION fail_admission_test_audit()`)
	require.NoError(t, err)
	_, err = f.service.Admit(t.Context(), "rp", "")
	require.Error(t, err)
	require.Zero(t, count(t, f, "SELECT count(*) FROM admin_admissions"))
	require.Zero(t, count(t, f, "SELECT count(*) FROM auth_identity_links WHERE external_subject='alice'"))
	require.Equal(t, 1, count(t, f, "SELECT count(*) FROM auth_subjects"))
}

func TestPostgresAdmissionRechecksRevokedGrantAndChangedRole(t *testing.T) {
	for _, changedRole := range []bool{false, true} {
		t.Run(map[bool]string{false: "issuer-revoked", true: "role-expanded"}[changedRole], func(t *testing.T) {
			f := setup(t)
			ctx := t.Context()
			i, err := f.service.Invite(ctx, f.actor.Subject.ID.String(), "alice", []int64{f.role.ID}, f.now.Add(time.Hour))
			require.NoError(t, err)
			if changedRole {
				key := rbac.MustPermissionKey("admission-test", "write")
				_, err = f.roles.UpsertPermission(ctx, rbac.Permission{Key: key})
				require.NoError(t, err)
				require.NoError(t, f.roles.ReplaceRolePermissions(ctx, f.role.ID, []rbac.PermissionKey{key}))
			} else {
				require.NoError(t, f.roles.ReplaceSubjectRoles(ctx, f.actor.Subject.ID, nil))
			}
			_, err = f.service.Admit(ctx, "rp", i.ID)
			require.True(t, errors.Is(err, admission.ErrDenied) || errors.Is(err, admission.ErrConflict))
			require.Zero(t, count(t, f, "SELECT count(*) FROM admin_admissions"))
			require.Zero(t, count(t, f, "SELECT count(*) FROM admin_admission_invitations WHERE accepted_subject IS NOT NULL"))
		})
	}
}

func TestPostgresAdmissionNeverRestoresSoftDeletedMembership(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	i, err := f.service.Invite(ctx, f.actor.Subject.ID.String(), "alice", []int64{f.role.ID}, f.now.Add(time.Hour))
	require.NoError(t, err)
	r, err := f.service.Admit(ctx, "rp", i.ID)
	require.NoError(t, err)
	_, err = f.auth.Runtime().Database().ExecContext(ctx,
		"UPDATE administrations SET deleted_at=now() WHERE subject_id=$1", r.LocalSubject)
	require.NoError(t, err)
	_, err = f.service.Admit(ctx, "rp", i.ID)
	require.ErrorIs(t, err, admission.ErrDenied)
	require.Zero(t, count(t, f, "SELECT count(*) FROM administrations WHERE subject_id=$1 AND deleted_at IS NULL", r.LocalSubject))
}
