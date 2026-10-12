//nolint:testpackage // Verifies the private borrowed-runtime policy conflict guard.
package host

import (
	"testing"

	"github.com/assurrussa/goauth"
	"github.com/stretchr/testify/require"
)

func TestBorrowedAuthMFAPolicyMustMatchExplicitHostPolicy(t *testing.T) {
	for _, actual := range []goauth.MFAMode{goauth.MFAModeOptional, goauth.MFAModeRequired} {
		require.NoError(t, validateBorrowedMFAMode("", actual))
		require.NoError(t, validateBorrowedMFAMode(actual, actual))
		require.Error(t, validateBorrowedMFAMode("invalid", actual))
	}
	require.Error(t, validateBorrowedMFAMode(goauth.MFAModeRequired, goauth.MFAModeOptional))
	require.Error(t, validateBorrowedMFAMode(goauth.MFAModeOptional, goauth.MFAModeRequired))
}
