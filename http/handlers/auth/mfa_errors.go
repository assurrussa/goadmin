package auth

import (
	"errors"
	"github.com/assurrussa/goadmin/services/adminservice"
	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
)

func mfaManagementFailure(err error) (int, string, string) {
	switch {
	case errors.Is(err, goauth.ErrOperationOutcomeUnknown):
		return fiber.StatusServiceUnavailable, "outcome_unknown", "The operation outcome is unknown. Sign in again and check protection before repeating."
	case errors.Is(err, adminservice.ErrLocalMFAUnavailable), errors.Is(err, goauth.ErrMFAUnavailable):
		return fiber.StatusServiceUnavailable, "mfa_unavailable", "Local MFA management is unavailable. Existing protection is not reported as disabled."
	case errors.Is(err, goauth.ErrSessionRevoked), errors.Is(err, goauth.ErrExpiredToken), errors.Is(err, goauth.ErrInvalidToken), errors.Is(err, goauth.ErrSecurityVersionMismatch), errors.Is(err, goauth.ErrMembershipDenied), errors.Is(err, goauth.ErrAccountUnavailable):
		return fiber.StatusUnauthorized, "signin_required", "Your session has ended or changed. Sign in again."
	case errors.Is(err, goauth.ErrAuthenticationRateLimited):
		return fiber.StatusTooManyRequests, "rate_limited", "Too many verification attempts. Wait before trying again."
	case errors.Is(err, goauth.ErrCurrentPasswordInvalid), errors.Is(err, goauth.ErrInvalidCredentials):
		return fiber.StatusBadRequest, "proof_invalid", "Check your current password and a fresh verification code."
	case errors.Is(err, goauth.ErrMFALoginDenied):
		// GoAuth deliberately uses this denial for both bad codes and stale setup/session.
		return fiber.StatusBadRequest, "verification_rejected", "Verification was rejected. Check fresh proof, or sign in again if setup or session changed."
	default:
		return fiber.StatusServiceUnavailable, "service_unavailable", "The service could not confirm this operation. Check protection before repeating."
	}
}
