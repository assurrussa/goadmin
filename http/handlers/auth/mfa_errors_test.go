package auth //nolint:testpackage // Verifies safe HTTP error classification.

import (
	"errors"
	"testing"

	"github.com/assurrussa/goauth"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/services/adminservice"
)

func TestMFAManagementFailureSeparatesOperationalAndVerificationErrors(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
		code   string
	}{
		{goauth.ErrCurrentPasswordInvalid, 400, "proof_invalid"},
		{goauth.ErrMFALoginDenied, 400, "verification_rejected"},
		{goauth.ErrSessionRevoked, 401, "signin_required"},
		{goauth.ErrExpiredToken, 401, "signin_required"},
		{goauth.ErrAuthenticationRateLimited, 429, "rate_limited"},
		{goauth.ErrMFAUnavailable, 503, "mfa_unavailable"},
		{errors.Join(adminservice.ErrLocalMFAUnavailable, goauth.ErrSessionRevoked), 503, "mfa_unavailable"},
		{goauth.ErrOperationOutcomeUnknown, 503, "outcome_unknown"},
		{errors.New("private database details"), 503, "service_unavailable"},
	} {
		status, code, message := mfaManagementFailure(test.err)
		require.Equal(t, test.status, status)
		require.Equal(t, test.code, code)
		require.NotContains(t, message, "private database")
	}
}
