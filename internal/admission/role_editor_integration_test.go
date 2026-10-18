//go:build integration

package admission_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	admission "github.com/assurrussa/goadmin/internal/admission"
)

// Pause after the real grant authorization, while its database locks are held.
type roleEditorBarrierStore struct {
	admission.Store
	checked chan struct{}
	resume  <-chan struct{}
	once    sync.Once
}

func (s *roleEditorBarrierStore) Owned(ctx context.Context, fn func(context.Context, admission.Tx) error) error {
	return s.Store.Owned(ctx, func(ctx context.Context, tx admission.Tx) error {
		return fn(ctx, roleEditorBarrierTx{Tx: tx, barrier: s})
	})
}

type roleEditorBarrierTx struct {
	admission.Tx
	barrier *roleEditorBarrierStore
}

func (tx roleEditorBarrierTx) Authorize(
	ctx context.Context, actor, action string, ids []int64,
) ([]admission.Role, error) {
	roles, err := tx.Tx.Authorize(ctx, actor, action, ids)
	if err != nil {
		return nil, err
	}
	tx.barrier.once.Do(func() { close(tx.barrier.checked) })
	select {
	case <-tx.barrier.resume:
		return roles, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestPostgresAdmissionActivationSerializesWithCanonicalRoleEditor(t *testing.T) {
	f := setup(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	invite, err := f.service.Invite(ctx, f.actor.Subject.ID.String(), "alice", []int64{f.role.ID}, f.now.Add(time.Hour))
	require.NoError(t, err)
	resume := make(chan struct{})
	var release sync.Once
	var workers sync.WaitGroup
	defer func() {
		cancel()
		release.Do(func() { close(resume) })
		workers.Wait()
	}()
	barrier := &roleEditorBarrierStore{Store: f.store, checked: make(chan struct{}), resume: resume}
	service, err := admission.New(admission.Config{
		Mode: admission.ProjectPending, Scope: f.proof.identity.Scope,
		Now: func() time.Time { return f.now }, NewID: uuid.NewString,
	}, barrier, f.proof)
	require.NoError(t, err)
	type admissionOutcome struct {
		record admission.Record
		err    error
	}
	admitted := make(chan admissionOutcome, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		record, err := service.Admit(ctx, "verified-role-editor-rp", invite.ID)
		admitted <- admissionOutcome{record: record, err: err}
	}()
	select {
	case <-barrier.checked:
	case <-ctx.Done():
		t.Fatal("admission did not reach real grant authorization")
	}
	editorPID := make(chan int, 1)
	edited := make(chan error, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		edited <- f.auth.Runtime().InOwnedAuthTransaction(ctx, func(txCtx context.Context) error {
			executor, err := f.auth.Runtime().SQLExecutor(txCtx)
			if err != nil {
				return err
			}
			var pid int
			if err := executor.QueryRowContext(txCtx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
				return err
			}
			editorPID <- pid
			role := f.role
			role.Name = "Edited admission reader"
			_, err = f.roles.UpdateRole(txCtx, role, nil)
			return err
		})
	}()
	var pid int
	select {
	case pid = <-editorPID:
	case <-ctx.Done():
		t.Fatal("canonical role editor did not start")
	}
	// Observe a real server-side lock wait before resuming activation. With the
	// old table fence the editor held the role tuple and waited for RowExclusive;
	// the activation's FK insert then formed a deadlock. The row fence instead
	// lets the editor acquire its relation lock and wait for the role tuple.
	require.Eventually(t, func() bool {
		var waiting bool
		err := f.auth.Runtime().Database().QueryRowContext(ctx, `SELECT EXISTS (
 SELECT 1 FROM pg_locks WHERE pid=$1 AND NOT granted)`, pid).Scan(&waiting)
		require.NoError(t, err)
		return waiting
	}, 5*time.Second, 10*time.Millisecond)
	release.Do(func() { close(resume) })
	var outcome admissionOutcome
	select {
	case outcome = <-admitted:
		require.NoError(t, outcome.err, "activation must not deadlock with UpdateRole")
	case <-ctx.Done():
		t.Fatal("activation did not finish")
	}
	select {
	case err := <-edited:
		require.NoError(t, err, "canonical UpdateRole must complete after activation")
	case <-ctx.Done():
		t.Fatal("canonical role editor did not finish")
	}
	require.Equal(t, admission.Active, outcome.record.Status)
	var accepted string
	require.NoError(t, f.auth.Runtime().Database().QueryRowContext(ctx,
		"SELECT accepted_subject::text FROM admin_admission_invitations WHERE id=$1", invite.ID).Scan(&accepted))
	require.Equal(t, outcome.record.LocalSubject, accepted)
	require.Equal(t, 1, count(t, f,
		"SELECT count(*) FROM administrations WHERE subject_id=$1", accepted))
	require.Equal(t, 1, count(t, f,
		"SELECT count(*) FROM auth_subject_roles WHERE subject_id=$1 AND role_id=$2", accepted, f.role.ID))
	current, err := f.roles.Role(ctx, f.role.ID)
	require.NoError(t, err)
	require.Equal(t, "Edited admission reader", current.Name)
	// One successful acceptance is stable under a repeated callback, too.
	repeated, err := service.Admit(ctx, "verified-role-editor-rp", invite.ID)
	require.NoError(t, err)
	require.Equal(t, outcome.record.LocalSubject, repeated.LocalSubject)
	require.Equal(t, 1, count(t, f,
		"SELECT count(*) FROM admin_admission_invitations WHERE id=$1 AND accepted_subject=$2", invite.ID, accepted))
}
