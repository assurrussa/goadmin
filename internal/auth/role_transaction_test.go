package auth //nolint:testpackage // checks the private canonical transaction bridge

import (
	"context"
	"errors"
	"testing"

	"github.com/assurrussa/goauth"
	"github.com/stretchr/testify/require"
)

func TestSubjectRoleTransactionOrdersAndDeduplicatesLocks(t *testing.T) {
	first := goauth.MustParseSubjectID("00000000-0000-4000-8000-000000000001")
	second := goauth.MustParseSubjectID("00000000-0000-4000-8000-000000000002")
	input := []string{second.String(), first.String(), second.String()}
	failure := errors.New("mutation failed")
	service := &RoleService{roleTransaction: func(
		ctx context.Context, subjects []goauth.SubjectID, fn func(context.Context) error,
	) error {
		require.Equal(t, []goauth.SubjectID{first, second}, subjects)
		return fn(ctx)
	}}
	err := service.InSubjectRoleTransaction(t.Context(), input, func(ctx context.Context) error {
		require.Equal(t, t.Context(), ctx)
		return failure
	})
	require.ErrorIs(t, err, failure)
	require.Equal(t, []string{second.String(), first.String(), second.String()}, input)
}

func TestSubjectRoleTransactionFailsClosed(t *testing.T) {
	called := false
	callback := func(context.Context) error { called = true; return nil }
	for _, service := range []*RoleService{nil, {}} {
		require.Error(t, service.InSubjectRoleTransaction(t.Context(), nil, callback))
	}
	service := &RoleService{roleTransaction: func(context.Context, []goauth.SubjectID, func(context.Context) error) error {
		t.Fatal("invalid subject reached the transaction")
		return nil
	}}
	require.ErrorIs(t, service.InSubjectRoleTransaction(t.Context(), []string{"not-a-subject"}, callback), ErrInvalidSubjectID)
	require.False(t, called)
}
