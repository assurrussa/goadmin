package adminservice

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"

	sessioncore "github.com/assurrussa/goadmin/infrastructure/core/session"
)

var ErrLocalMFAUnavailable = errors.New("local MFA management unavailable")

type mfaManagementRuntime interface {
	MFAStatus(ctx context.Context, auth goauth.AuthContext) (bool, error)
	BeginMFAEnrollment(ctx context.Context, request goauth.MFAEnrollmentRequest) (goauth.MFAEnrollment, error)
	ConfirmMFAEnrollment(ctx context.Context, request goauth.MFAEnrollmentConfirmRequest) ([]string, error)
	CancelMFAEnrollment(ctx context.Context, auth goauth.AuthContext, binding [32]byte) error
	DisableMFA(ctx context.Context, request goauth.MFAChangeRequest) error
	RegenerateMFARecovery(ctx context.Context, request goauth.MFAChangeRequest) ([]string, error)
}

func (s *Service) currentMFAAuth(c fiber.Ctx) (goauth.AuthContext, [32]byte, error) {
	var binding [32]byte
	admin, err := s.GetAdminAuth(c)
	if err != nil {
		return goauth.AuthContext{}, binding, err
	}
	if admin == nil {
		return goauth.AuthContext{}, binding, goauth.ErrSessionRevoked
	}
	sess, err := s.store.Get(c)
	if err != nil {
		return goauth.AuthContext{}, binding, err
	}
	defer sess.Release()
	state, ok := sess.Get(sessioncore.AuthAdminKey.String()).(*browserSession)
	if ok && state != nil && state.External != nil {
		return goauth.AuthContext{}, binding, errors.Join(ErrLocalMFAUnavailable, goauth.ErrSessionRevoked)
	}
	if !ok || state == nil {
		return goauth.AuthContext{}, binding, goauth.ErrSessionRevoked
	}
	active, err := s.loadBrowserAuthority(c, sess, state)
	if err != nil {
		return goauth.AuthContext{}, binding, err
	}
	if !active {
		return goauth.AuthContext{}, binding, goauth.ErrSessionRevoked
	}
	auth, err := s.runtime.VerifyAccessToken(c, state.AccessToken, true)
	if err != nil {
		return goauth.AuthContext{}, binding, err
	}
	if auth.SubjectID != admin.SubjectID || auth.Realm != goauth.RealmAdmin {
		return goauth.AuthContext{}, binding, goauth.ErrInvalidToken
	}
	binding = sha256.Sum256([]byte("goadmin/mfa-enrollment/v1:" + sess.ID()))
	return auth, binding, nil
}

func (s *Service) LocalMFAStatus(c fiber.Ctx) (bool, error) {
	rt, ok := s.runtime.(mfaManagementRuntime)
	if !ok {
		return false, ErrLocalMFAUnavailable
	}
	auth, _, err := s.currentMFAAuth(c)
	if err != nil {
		return false, err
	}
	return rt.MFAStatus(c, auth)
}

type MFAManagementInput struct {
	Password     string `json:"password"`
	Code         string `json:"code"`
	EnrollmentID string `json:"enrollmentID"` //nolint:tagliatelle // Matches the existing MFA settings request field.
}
type MFAEnrollmentView struct {
	ID        string    `json:"id"`
	Secret    string    `json:"secret"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expiresAt"`
}
type MFAManagementResult struct {
	Enrollment    *MFAEnrollmentView `json:"enrollment,omitempty"`
	RecoveryCodes []string           `json:"recoveryCodes,omitempty"`
	SignedOut     bool               `json:"signedOut"`
}

func (s *Service) ManageLocalMFA(c fiber.Ctx, action string, input MFAManagementInput) (MFAManagementResult, error) {
	var result MFAManagementResult
	rt, ok := s.runtime.(mfaManagementRuntime)
	if !ok {
		return result, ErrLocalMFAUnavailable
	}
	auth, binding, err := s.currentMFAAuth(c)
	if err != nil {
		return result, err
	}
	switch action {
	case "begin":
		enrollment, err := rt.BeginMFAEnrollment(c, goauth.MFAEnrollmentRequest{
			Auth: auth, Password: input.Password, CurrentCode: input.Code, Binding: binding,
		})
		if err != nil {
			return result, err
		}
		result.Enrollment = &MFAEnrollmentView{
			ID: enrollment.ID, Secret: enrollment.Secret, URL: enrollment.URL, ExpiresAt: enrollment.ExpiresAt,
		}
	case "confirm":
		result.RecoveryCodes, err = rt.ConfirmMFAEnrollment(c, goauth.MFAEnrollmentConfirmRequest{
			Auth: auth, Password: input.Password, EnrollmentID: input.EnrollmentID, Code: input.Code, Binding: binding,
		})
		result.SignedOut = err == nil
	case "cancel":
		err = rt.CancelMFAEnrollment(c, auth, binding)
	case "disable":
		err = rt.DisableMFA(c, goauth.MFAChangeRequest{Auth: auth, Password: input.Password, Code: input.Code})
		result.SignedOut = err == nil
	case "recovery":
		result.RecoveryCodes, err = rt.RegenerateMFARecovery(c, goauth.MFAChangeRequest{
			Auth: auth, Password: input.Password, Code: input.Code,
		})
		result.SignedOut = err == nil
	default:
		return result, goauth.ErrInvalidToken
	}
	if err != nil {
		return MFAManagementResult{}, err
	}
	// Canonical security version invalidates all sessions. Do not attempt to
	// preserve a stale one; leave one-time recovery codes in this response only.
	if result.SignedOut {
		s.deleteSessionCookies(c)
	}
	return result, nil
}
