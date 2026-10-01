package host //nolint:testpackage // verifies fail-closed guard behavior without exporting a test constructor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	integrationroles "github.com/assurrussa/goadmin/internal/auth"
)

func TestSubjectPermissionCheckerAllowsOnlyGuardedCanonicalSubject(t *testing.T) {
	t.Parallel()
	key := NewPermissionKey(PermissionDomain("cms-entry"), PermissionActionRead)
	guard := &subjectPermissionGuardStub{allowed: true}
	checker := &SubjectPermissionChecker{guard: guard}

	require.NoError(t, checker.CheckPermission(context.Background(), "subject-1", key))
	require.Equal(t, "subject-1", guard.subjectID)
	require.Equal(t, key, guard.key)

	guard.allowed = false
	require.ErrorIs(t, checker.CheckPermission(context.Background(), "subject-1", key), ErrSubjectPermissionDenied)
}

func TestSubjectPermissionCheckerFailsClosedBeforeGuard(t *testing.T) {
	t.Parallel()
	guard := &subjectPermissionGuardStub{allowed: true}
	checker := &SubjectPermissionChecker{guard: guard}
	require.ErrorIs(t, checker.CheckPermission(context.Background(), "", PermissionKey{}), ErrSubjectPermissionDenied)
	require.Zero(t, guard.calls)
	require.ErrorIs(t, (*SubjectPermissionChecker)(nil).CheckPermission(
		context.Background(), "subject-1", NewPermissionKey(PermissionDomain("cms-entry"), PermissionActionRead),
	), ErrSubjectPermissionDenied)
}

func TestNewSubjectPermissionCheckerStillValidatesRequiredInputs(t *testing.T) {
	t.Parallel()
	_, err := NewSubjectPermissionChecker(nil, nil, nil, nil) //nolint:staticcheck // validates the nil-context rejection path
	require.ErrorContains(t, err, "context is required")
	_, err = NewSubjectPermissionChecker(context.Background(), nil, nil, nil)
	require.ErrorContains(t, err, "PostgreSQL pool is required")
}

type subjectPermissionGuardStub struct {
	allowed   bool
	calls     int
	subjectID string
	key       PermissionKey
}

func (s *subjectPermissionGuardStub) SubjectGuardCheck(
	_ context.Context,
	subjectID string,
	key integrationroles.PermissionKey,
	_ ...integrationroles.PermissionGuardOption,
) bool {
	s.calls++
	s.subjectID = subjectID
	s.key = key
	return s.allowed
}
