package adminservice

import (
	"context"
	"errors"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/internal/auth/browsercookie"
	"github.com/assurrussa/goadmin/models"
)

// MFALoginEnrollmentView contains inactive one-time recovery codes. Only the
// matching setup confirmation activates them; this value is never persisted.
type MFALoginEnrollmentView struct {
	MFAEnrollmentView
	RecoveryCodes []string `json:"recoveryCodes"`
}

type localMFAEnrollmentRuntime interface {
	BeginLocalLoginEnrollment(ctx context.Context, pendingID string, binding [32]byte) (goauth.MFAEnrollment, error)
	CompleteLocalLoginEnrollment(
		ctx context.Context, pendingID, enrollmentID string, binding [32]byte, code string,
	) (goauth.LoginResult, error)
}

// claimLocalEnrollment never accepts an authenticated browser session as setup
// authority. Only the password-verified, browser-bound pending flow can proceed.
func (s *Service) claimLocalEnrollment(
	ctx context.Context, c fiber.Ctx, challengeID string,
) (localLoginClaim, error) {
	if challengeID == "" {
		return localLoginClaim{}, goauth.ErrInvalidToken
	}
	if err := browsercookie.Check(c, s.store); err != nil {
		return localLoginClaim{}, err
	}
	sess, err := s.store.Get(c)
	if err != nil {
		return localLoginClaim{}, err
	}
	id := sess.ID()
	sess.Release()
	claim, err := s.claimLocalLogin(ctx, id, challengeID)
	if err != nil {
		return claim, err
	}
	if claim.record.LocalLoginPending.Kind != goauth.MFALoginKindEnrollment {
		_ = s.commitLocalLogin(ctx, claim, claim.record)
		return claim, goauth.ErrInvalidToken
	}
	return claim, nil
}

func (s *Service) BeginLocalAdminEnrollment(c fiber.Ctx, challengeID string) (MFALoginEnrollmentView, error) {
	rt, ok := s.runtime.(localMFAEnrollmentRuntime)
	if !ok {
		return MFALoginEnrollmentView{}, errors.New("local MFA enrollment runtime unavailable")
	}
	ctx, cancel := context.WithTimeout(c.Context(), localLoginBudget)
	defer cancel()
	claim, err := s.claimLocalEnrollment(ctx, c, challengeID)
	if err != nil {
		return MFALoginEnrollmentView{}, err
	}
	pending := claim.record.LocalLoginPending
	enrollment, err := rt.BeginLocalLoginEnrollment(ctx, pending.ID, pending.Binding)
	if err != nil {
		// A failed/unknown begin cannot expose a possibly superseded secret.
		// Keep ownership pinned; returning to login explicitly abandons it.
		return MFALoginEnrollmentView{}, err
	}
	if enrollment.ID == "" || enrollment.Secret == "" || len(enrollment.RecoveryCodes) == 0 ||
		!time.Now().Before(enrollment.ExpiresAt) {
		return MFALoginEnrollmentView{}, goauth.ErrInvalidToken
	}
	if err := s.commitLocalLogin(ctx, claim, claim.record); err != nil {
		return MFALoginEnrollmentView{}, err
	}
	return MFALoginEnrollmentView{
		MFAEnrollmentView: MFAEnrollmentView{
			ID: enrollment.ID, Secret: enrollment.Secret, URL: enrollment.URL, ExpiresAt: enrollment.ExpiresAt,
		},
		RecoveryCodes: enrollment.RecoveryCodes,
	}, nil
}

func (s *Service) CompleteLocalAdminEnrollment(
	c fiber.Ctx, challengeID, enrollmentID, code string,
) (models.Admin, goauth.Account, error) {
	rt, ok := s.runtime.(localMFAEnrollmentRuntime)
	if !ok {
		return models.Admin{}, goauth.Account{}, errors.New("local MFA enrollment runtime unavailable")
	}
	ctx, cancel := context.WithTimeout(c.Context(), localLoginBudget)
	defer cancel()
	claim, err := s.claimLocalEnrollment(ctx, c, challengeID)
	if err != nil {
		return models.Admin{}, goauth.Account{}, err
	}
	pending := claim.record.LocalLoginPending
	result, err := rt.CompleteLocalLoginEnrollment(ctx, pending.ID, enrollmentID, pending.Binding, code)
	if err != nil {
		if errors.Is(err, goauth.ErrMFALoginDenied) || errors.Is(err, goauth.ErrAuthenticationRateLimited) {
			_ = s.commitLocalLogin(ctx, claim, claim.record)
		}
		return models.Admin{}, goauth.Account{}, err
	}
	return s.finishClaimedLocalAdmin(ctx, c, claim, result, pending.Persistent)
}
