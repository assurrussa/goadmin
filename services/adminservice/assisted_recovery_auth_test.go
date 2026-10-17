package adminservice //nolint:testpackage // Exercises the internal canonical runtime adapter.

import (
	"testing"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/testkit"
	"github.com/stretchr/testify/require"
)

func TestFindRecoveryAccountUsesCanonicalAccountWithoutUserProjection(t *testing.T) {
	fixture, err := testkit.NewRuntime()
	require.NoError(t, err)
	account, err := fixture.Runtime.ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{Email: "recovery-target@example.test", Password: "Synthetic-Password-42"})
	require.NoError(t, err)
	service := NewService(nil, nil, WithRuntime(fixture.Runtime, adminRepositoryStub{}))
	found, err := service.FindRecoveryAccount(t.Context(), "recovery-target@example.test")
	require.NoError(t, err)
	require.Equal(t, account.Subject.ID, found.Subject.ID)
	_, err = service.FindRecoveryAccount(t.Context(), "missing@example.test")
	require.ErrorIs(t, err, goauth.ErrAccountNotFound)
}
