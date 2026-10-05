//go:build integration

package roles_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	integrationroles "github.com/assurrussa/goadmin/internal/auth"
)

const (
	roleMutationCommitUnknown  = "commit_outcome_unknown"
	roleMutationCommitRejected = "commit_rejected"
)

type roleMutationTransactionFunc func(context.Context, int64, int64, func(context.Context) error) error

func (fn roleMutationTransactionFunc) InAdminRoleTransaction(
	ctx context.Context, actorID, targetID int64, callback func(context.Context) error,
) error {
	return fn(ctx, actorID, targetID, callback)
}

func TestAdminRoleMutationHTTPTransactionFailure(t *testing.T) {
	for _, action := range []string{roleMutationAttach, roleMutationDetach} {
		for _, failure := range []string{"callback_rollback", roleMutationCommitRejected, roleMutationCommitUnknown} {
			t.Run(action+"/"+failure, func(t *testing.T) {
				runRoleMutationTransactionFailure(t, action, failure)
			})
		}
	}
}

func runRoleMutationTransactionFailure(t *testing.T, action, failure string) {
	t.Helper()
	f := newRoleMutationFixture(t)
	if action == roleMutationDetach {
		f.setTargetRoles(t, f.base.ID, f.first.ID)
	}
	before := f.targetRoleIDs(t)
	useCases := f.useCases
	calls, bodyBeforeCommit := 0, -1
	var transactionError, injectedSQLError error
	useCases.AdminRoleTransaction = roleMutationTransactionFunc(func(
		ctx context.Context, actorID, targetID int64, callback func(context.Context) error,
	) error {
		calls++
		transaction := f.useCases.AdminRoleTransaction
		transactionError = transaction.InAdminRoleTransaction(ctx, actorID, targetID, func(txCtx context.Context) error {
			if err := callback(txCtx); err != nil {
				return err
			}
			fiberCtx, ok := ctx.(fiber.Ctx)
			if !ok {
				return errors.New("role handler must pass its Fiber context")
			}
			bodyBeforeCommit = len(fiberCtx.Response().Body())
			switch failure {
			case "callback_rollback":
				return errors.New("injected error after canonical role replacement")
			case roleMutationCommitRejected:
				executor, err := f.auth.Runtime().SQLExecutor(txCtx)
				if err != nil {
					return err
				}
				// PostgreSQL marks the transaction aborted. Return nil so
				// the real outer COMMIT, not this callback, rejects it.
				_, injectedSQLError = executor.ExecContext(txCtx, "SELECT 1 / 0")
			}
			return nil
		})
		if failure == roleMutationCommitUnknown && transactionError == nil {
			// Model loss of the commit acknowledgement after a real
			// commit. The HTTP layer must not retry an ambiguous write.
			transactionError = fmt.Errorf("commit acknowledgement: %w", goauth.ErrOperationOutcomeUnknown)
		}
		return transactionError
	})
	result := requestRoleMutation(f.httpApp(t, useCases), f.target.ID, f.first.ID, action)
	requireRoleMutationError(t, result, http.StatusInternalServerError)
	require.Equal(t, 1, calls, "a commit error must not replay the role mutation")
	require.Zero(t, bodyBeforeCommit, "do not prepare an HTTP success before canonical commit")
	require.Error(t, transactionError)
	if failure == roleMutationCommitRejected {
		require.Error(t, injectedSQLError, "test must reach a real PostgreSQL transaction abort")
		require.Contains(t, transactionError.Error(), "commit auth transaction")
	}
	if failure != roleMutationCommitUnknown {
		require.Equal(t, before, f.targetRoleIDs(t), "failed transaction must preserve the old assignments")
		return
	}
	require.ErrorIs(t, transactionError, goauth.ErrOperationOutcomeUnknown)
	want := []int64{f.base.ID}
	if action == roleMutationAttach {
		want = append(want, f.first.ID)
	}
	require.Equal(t, want, f.targetRoleIDs(t), "unknown outcome must be reconciled, never compensated or replayed")
}

func TestAdminRoleMutationHTTPSuccessWaitsForCommit(t *testing.T) {
	for _, action := range []string{roleMutationAttach, roleMutationDetach} {
		t.Run(action, func(t *testing.T) {
			f := newRoleMutationFixture(t)
			if action == roleMutationDetach {
				f.setTargetRoles(t, f.base.ID, f.first.ID)
			}
			before := f.targetRoleIDs(t)
			beforeCommit, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(release) })
			bodyBeforeCommit := -1
			useCases := f.useCases
			useCases.AdminRoleTransaction = roleMutationTransactionFunc(func(
				ctx context.Context, actorID, targetID int64, callback func(context.Context) error,
			) error {
				return f.useCases.AdminRoleTransaction.InAdminRoleTransaction(ctx, actorID, targetID, func(txCtx context.Context) error {
					if err := callback(txCtx); err != nil {
						return err
					}
					fiberCtx, ok := ctx.(fiber.Ctx)
					if !ok {
						return errors.New("role handler must pass its Fiber context")
					}
					bodyBeforeCommit = len(fiberCtx.Response().Body())
					close(beforeCommit)
					<-release
					return nil
				})
			})
			app := f.httpApp(t, useCases)
			result := make(chan roleMutationHTTPResult, 1)
			go func() { result <- requestRoleMutation(app, f.target.ID, f.first.ID, action) }()
			awaitRoleMutationSignal(t, beforeCommit)
			require.Zero(t, bodyBeforeCommit)
			require.Equal(t, before, f.targetRoleIDs(t), "independent readers must not see an uncommitted replacement")
			select {
			case early := <-result:
				t.Fatalf("HTTP response escaped before commit: %+v", early)
			default:
			}
			releaseOnce.Do(func() { close(release) })
			requireRoleMutationStatus(t, <-result, http.StatusOK)
			want := []int64{f.base.ID}
			if action == roleMutationAttach {
				want = append(want, f.first.ID)
			}
			require.Equal(t, want, f.targetRoleIDs(t))
		})
	}
}

func TestAdminRoleMutationHTTPRechecksProtectedRolesAfterLock(t *testing.T) {
	for _, changingSubject := range []string{"actor", "target"} {
		t.Run(changingSubject, func(t *testing.T) {
			f := newRoleMutationFixture(t)
			super := f.createRole(t, integrationroles.SuperAdminRole)
			subjectID, changedIDs := f.actor.SubjectID.String(), []int64{f.operator.ID}
			mutationRoleID := super.ID
			wantTarget := []int64{f.base.ID}
			if changingSubject == "actor" {
				require.NoError(t, f.auth.Roles().AssignRolesToSubject(t.Context(), subjectID, []int64{f.operator.ID, super.ID}))
			} else {
				subjectID, changedIDs = f.target.SubjectID.String(), []int64{f.base.ID, super.ID}
				mutationRoleID = f.first.ID
				wantTarget = changedIDs
			}
			changed, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(release) })
			externalResult := make(chan error, 1)
			go func() {
				transaction := f.auth.Roles().InSubjectRoleTransaction
				externalResult <- transaction(t.Context(), []string{subjectID}, func(txCtx context.Context) error {
					if err := f.auth.Roles().AssignRolesToSubject(txCtx, subjectID, changedIDs); err != nil {
						return err
					}
					close(changed)
					<-release
					return nil
				})
			}()
			awaitRoleMutationSignal(t, changed)
			app := f.httpApp(t, f.useCases)
			result, returned := make(chan roleMutationHTTPResult, 1), make(chan struct{})
			go func() {
				result <- requestRoleMutation(app, f.target.ID, mutationRoleID, roleMutationAttach)
				close(returned)
			}()
			awaitRoleMutationContention(t, f, returned)
			releaseOnce.Do(func() { close(release) })
			require.NoError(t, <-externalResult)
			requireRoleMutationError(t, <-result, http.StatusForbidden)
			require.Equal(t, wantTarget, f.targetRoleIDs(t))
		})
	}
}

func TestAdminRoleMutationReciprocalHTTP(t *testing.T) {
	f := newRoleMutationFixture(t)
	f.setTargetRoles(t, f.base.ID, f.operator.ID)
	other := *f
	other.actor, other.target = f.target, f.actor
	ready, release := make(chan struct{}, 2), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	useCases := f.useCases
	useCases.AdminRoleTransaction = roleMutationTransactionFunc(func(
		ctx context.Context, actorID, targetID int64, callback func(context.Context) error,
	) error {
		// Both authenticated requests reach the transaction boundary before
		// either takes a lock. Reversing actor and target must retain the same
		// canonical lock order instead of acquiring mutually opposing locks.
		ready <- struct{}{}
		<-release
		return f.useCases.AdminRoleTransaction.InAdminRoleTransaction(ctx, actorID, targetID, callback)
	})
	firstApp, secondApp := f.httpApp(t, useCases), other.httpApp(t, useCases)
	results := make(chan roleMutationHTTPResult, 2)
	go func() { results <- requestRoleMutation(firstApp, f.target.ID, f.first.ID, roleMutationAttach) }()
	go func() { results <- requestRoleMutation(secondApp, other.target.ID, f.second.ID, roleMutationAttach) }()
	awaitRoleMutationSignal(t, ready)
	awaitRoleMutationSignal(t, ready)
	releaseOnce.Do(func() { close(release) })
	for range 2 {
		requireRoleMutationStatus(t, <-results, http.StatusOK)
	}
	require.Equal(t, []int64{f.base.ID, f.first.ID, f.operator.ID}, f.targetRoleIDs(t))
	require.Equal(t, []int64{f.second.ID, f.operator.ID}, other.targetRoleIDs(t))
}
