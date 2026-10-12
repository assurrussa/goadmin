package adminservice

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/google/uuid"

	sessioncore "github.com/assurrussa/goadmin/infrastructure/core/session"
	"github.com/assurrussa/goadmin/internal/auth/browsercookie"
	"github.com/assurrussa/goadmin/internal/auth/browserstate"
	"github.com/assurrussa/goadmin/models"
)

var ErrLocalMFARequired = errors.New("local second factor required")

const (
	localMFAKey         = "goadmin.local-mfa-pending"
	localMFAReminderKey = "goadmin.local-mfa-reminder"
	localLoginBudget    = 8 * time.Second
)

type localMFARuntime interface {
	BeginLocalLogin(ctx context.Context, request goauth.LoginRequest, binding [32]byte) (goauth.LocalLoginResult, error)
	CompleteLocalLogin(ctx context.Context, id string, binding [32]byte, code string) (goauth.LoginResult, error)
}
type localLoginClaim struct {
	key    string
	owner  string
	record browserstate.Record
}

// Local-login owners are never stolen or leased. The existing raw journal is
// used deliberately: refresh-specific read filtering must not hide this state.
func (s *Service) claimLocalLogin(ctx context.Context, id, challengeID string) (localLoginClaim, error) {
	var claim localLoginClaim
	if s.localLoginStates == nil || id == "" {
		return claim, errors.New("atomic local login journal required")
	}
	claim.key = "local-login:" + id
	r, found, err := s.localLoginStates.Load(ctx, claim.key)
	if err != nil {
		return claim, err
	}
	if !found {
		if challengeID != "" {
			return claim, goauth.ErrExpiredToken
		}
		r = browserstate.Record{LocalLoginUntil: time.Now().Add(s.sessionTTL)}
		if err := s.localLoginStates.Create(ctx, claim.key, r, s.sessionTTL); err != nil {
			return claim, err
		}
		r, found, err = s.localLoginStates.Load(ctx, claim.key)
		if err != nil {
			return claim, err
		}
		if !found {
			return claim, goauth.ErrOperationOutcomeUnknown
		}
	}
	if r.LocalLoginTerminal || r.Owner != "" || !time.Now().Before(r.LocalLoginUntil) {
		return claim, goauth.ErrSessionRevoked
	}
	if challengeID != "" && (r.LocalLoginPending == nil ||
		r.LocalLoginPending.ID != challengeID || !time.Now().Before(r.LocalLoginPending.ExpiresAt)) {
		return claim, goauth.ErrExpiredToken
	}
	claim.owner = "local-login:" + uuid.NewString()
	claimed, err := s.localLoginStates.Claim(ctx, claim.key, r.Version, claim.owner)
	if err != nil {
		return claim, err
	}
	if !claimed {
		return claim, ErrRefreshInProgress
	}
	claim.record = r
	return claim, nil
}

func (s *Service) commitLocalLogin(ctx context.Context, claim localLoginClaim, record browserstate.Record) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ok, err := s.localLoginStates.Complete(ctx, claim.key, claim.record.Version, claim.owner, record)
	if err != nil {
		return err
	}
	if !ok {
		return goauth.ErrOperationOutcomeUnknown
	}
	// A store can commit after cancellation; never publish that late result.
	return ctx.Err()
}

func (s *Service) beginLocalAdmin(c fiber.Ctx, email, password string, persistent bool) (models.Admin, goauth.Account, error) {
	rt, ok := s.runtime.(localMFARuntime)
	if !ok {
		return models.Admin{}, goauth.Account{}, errors.New("local MFA runtime unavailable")
	}
	if err := browsercookie.Check(c, s.store); err != nil {
		return models.Admin{}, goauth.Account{}, err
	}
	ctx, cancel := context.WithTimeout(c.Context(), localLoginBudget)
	defer cancel()
	sess, err := s.store.Get(c)
	if err != nil {
		return models.Admin{}, goauth.Account{}, err
	}
	if state, ok := sess.Get(sessioncore.AuthAdminKey.String()).(*browserSession); ok && state != nil {
		sess.Release()
		return models.Admin{}, goauth.Account{}, goauth.ErrInvalidCredentials
	}
	id := sess.ID()
	// Keep the CSRF-bound anonymous ID stable until authentication. The journal's
	// version supplies the replacement fence; final authentication rotates the ID.
	err = sess.SaveWithContext(ctx)
	sess.Release()
	// Fiber sets its response cookie before saving; remove it even on failure.
	// Pending MFA and authenticated success publish their own cookie explicitly.
	s.discardStaleSessionCookies(c)
	if err != nil {
		return models.Admin{}, goauth.Account{}, err
	}
	claim, err := s.claimLocalLogin(ctx, id, "")
	if err != nil {
		return models.Admin{}, goauth.Account{}, err
	}
	var binding [32]byte
	if _, err := rand.Read(binding[:]); err != nil {
		return models.Admin{}, goauth.Account{}, err
	}
	result, err := rt.BeginLocalLogin(ctx, goauth.LoginRequest{
		Credential: goauth.Credential{
			Identifier: goauth.IdentifierInput{Scheme: goauth.IdentifierSchemeEmail, Value: email},
			Password:   password,
		},
		Realm: goauth.RealmAdmin,
	}, binding)
	if err != nil {
		if errors.Is(err, goauth.ErrInvalidCredentials) {
			claim.record.LocalLoginPending = nil
			_ = s.commitLocalLogin(ctx, claim, claim.record)
		}
		return models.Admin{}, goauth.Account{}, err
	}
	if result.Pending != nil {
		if result.Pending.ID == "" || !time.Now().Before(result.Pending.ExpiresAt) {
			return models.Admin{}, goauth.Account{}, goauth.ErrInvalidToken
		}
		claim.record.LocalLoginPending = &browserstate.LocalLoginPending{
			ID: result.Pending.ID, Kind: result.Pending.Kind, Binding: binding,
			ExpiresAt: result.Pending.ExpiresAt, Persistent: persistent,
		}
		if err := s.commitLocalLogin(ctx, claim, claim.record); err != nil {
			return models.Admin{}, goauth.Account{}, err
		}
		s.writeSessionCookie(c, id, false)
		return models.Admin{}, goauth.Account{}, ErrLocalMFARequired
	}
	return s.finishClaimedLocalAdmin(ctx, c, claim, result.Login, persistent)
}

type LocalMFAPending struct {
	ID        string
	Kind      goauth.MFALoginKind
	ExpiresAt time.Time
}

func (s *Service) LocalMFAChallenge(c fiber.Ctx) (string, time.Time, error) {
	pending, err := s.PendingLocalMFA(c)
	return pending.ID, pending.ExpiresAt, err
}

func (s *Service) PendingLocalMFA(c fiber.Ctx) (LocalMFAPending, error) {
	if s.localLoginStates == nil {
		return LocalMFAPending{}, errors.New("atomic local login journal required")
	}
	if err := browsercookie.Check(c, s.store); err != nil {
		return LocalMFAPending{}, err
	}
	sess, err := s.store.Get(c)
	if err != nil {
		return LocalMFAPending{}, err
	}
	id := sess.ID()
	sess.Release()
	r, found, err := s.localLoginStates.Load(c, "local-login:"+id)
	if err != nil {
		return LocalMFAPending{}, err
	}
	if !found || r.Owner != "" || r.LocalLoginTerminal || r.LocalLoginPending == nil ||
		!time.Now().Before(r.LocalLoginUntil) || !time.Now().Before(r.LocalLoginPending.ExpiresAt) {
		return LocalMFAPending{}, goauth.ErrExpiredToken
	}
	return LocalMFAPending{
		ID: r.LocalLoginPending.ID, Kind: r.LocalLoginPending.Kind,
		ExpiresAt: r.LocalLoginPending.ExpiresAt,
	}, nil
}

func (s *Service) CompleteLocalAdmin(c fiber.Ctx, challengeID, code string) (models.Admin, goauth.Account, error) {
	rt, ok := s.runtime.(localMFARuntime)
	if !ok {
		return models.Admin{}, goauth.Account{}, errors.New("local MFA runtime unavailable")
	}
	if challengeID == "" {
		return models.Admin{}, goauth.Account{}, goauth.ErrInvalidToken
	}
	if err := browsercookie.Check(c, s.store); err != nil {
		return models.Admin{}, goauth.Account{}, err
	}
	ctx, cancel := context.WithTimeout(c.Context(), localLoginBudget)
	defer cancel()
	sess, err := s.store.Get(c)
	if err != nil {
		return models.Admin{}, goauth.Account{}, err
	}
	id := sess.ID()
	sess.Release()
	claim, err := s.claimLocalLogin(ctx, id, challengeID)
	if err != nil {
		return models.Admin{}, goauth.Account{}, err
	}
	pending := *claim.record.LocalLoginPending
	if pending.Kind == goauth.MFALoginKindEnrollment {
		_ = s.commitLocalLogin(ctx, claim, claim.record)
		return models.Admin{}, goauth.Account{}, goauth.ErrInvalidToken
	}
	result, err := rt.CompleteLocalLogin(ctx, pending.ID, pending.Binding, code)
	if err != nil {
		// Only a definite incorrect factor permits retry. Uncertain outcomes retain
		// their non-reclaimable owner and never replay a possibly consumed factor.
		if errors.Is(err, goauth.ErrMFALoginDenied) || errors.Is(err, goauth.ErrAuthenticationRateLimited) {
			_ = s.commitLocalLogin(ctx, claim, claim.record)
		}
		return models.Admin{}, goauth.Account{}, err
	}
	return s.finishClaimedLocalAdmin(ctx, c, claim, result, pending.Persistent)
}

func (s *Service) finishClaimedLocalAdmin(
	ctx context.Context,
	c fiber.Ctx,
	claim localLoginClaim,
	result goauth.LoginResult,
	persistent bool,
) (models.Admin, goauth.Account, error) {
	publishedID := ""
	revoke := func() {
		// Compensation survives the operation deadline, but is independently bounded.
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		_ = s.runtime.Logout(cleanup, result.Account.Subject.ID, result.Tokens.Session.ID)
		if publishedID != "" {
			// Regeneration does not update the request cookie. Delete the exact
			// rotated projection, never a session reopened from that old cookie.
			if s.states != nil {
				_ = s.states.Delete(cleanup, publishedID)
			}
			_ = s.store.Delete(cleanup, publishedID)
		}
		// A late failure must not overwrite a newer response with an old cookie.
		s.discardStaleSessionCookies(c)
	}
	if !time.Now().Before(result.Tokens.Session.ExpiresAt) {
		revoke()
		return models.Admin{}, goauth.Account{}, goauth.ErrExpiredToken
	}
	auth, err := s.runtime.VerifyAccessToken(ctx, result.Tokens.AccessToken, true)
	if err != nil {
		revoke()
		return models.Admin{}, goauth.Account{}, err
	}
	if auth.Realm != goauth.RealmAdmin || auth.SubjectID != result.Account.Subject.ID || auth.SessionID != result.Tokens.Session.ID {
		revoke()
		return models.Admin{}, goauth.Account{}, goauth.ErrInvalidToken
	}
	remind := false
	if claim.record.LocalLoginPending == nil {
		if management, ok := s.runtime.(mfaManagementRuntime); ok {
			enabled, statusErr := management.MFAStatus(ctx, auth)
			remind = statusErr == nil && !enabled
		}
	}
	claim.record.LocalLoginPending = nil
	claim.record.LocalLoginTerminal = true
	claim.record.LocalLoginSessionID = result.Tokens.Session.ID
	if err := s.commitLocalLogin(ctx, claim, claim.record); err != nil {
		revoke()
		return models.Admin{}, goauth.Account{}, err
	}
	if s.localAdmission != nil {
		if err := s.localAdmission(ctx, result.Account.Subject.ID); err != nil {
			revoke()
			return models.Admin{}, goauth.Account{}, err
		}
	}
	admin, err := s.admins.GetBySubjectID(ctx, result.Account.Subject.ID)
	if err != nil || admin.ID <= 0 {
		revoke()
		if err == nil {
			err = goauth.ErrMembershipDenied
		}
		return models.Admin{}, goauth.Account{}, err
	}
	if err := ctx.Err(); err != nil {
		revoke()
		return models.Admin{}, goauth.Account{}, err
	}
	if !time.Now().Before(claim.record.LocalLoginUntil) {
		revoke()
		return models.Admin{}, goauth.Account{}, goauth.ErrExpiredToken
	}
	publishedID, err = s.saveAuthenticatedSessionWithLocalFlow(ctx, c, admin, result.Account, result.Tokens, persistent, claim.key)
	if err != nil {
		revoke()
		return models.Admin{}, goauth.Account{}, err
	}
	if err := s.checkLocalLoginPublication(ctx, claim.key, result.Tokens.Session.ID); err != nil {
		revoke()
		return models.Admin{}, goauth.Account{}, err
	}
	c.Locals(localMFAReminderKey, remind)
	return admin, result.Account, nil
}

func (s *Service) checkLocalLoginPublication(ctx context.Context, key, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.localLoginStates == nil {
		return goauth.ErrSessionRevoked
	}
	r, found, err := s.localLoginStates.Load(ctx, key)
	if err != nil {
		return err
	}
	if !found || r.Owner != "" || !r.LocalLoginTerminal || sessionID == "" ||
		r.LocalLoginSessionID != sessionID || !time.Now().Before(r.LocalLoginUntil) {
		return goauth.ErrSessionRevoked
	}
	return ctx.Err()
}

func (s *Service) validateLocalLoginPublication(ctx context.Context, state *browserSession) error {
	if state.External != nil || state.LocalLoginFlowKey == "" {
		return nil
	}
	return s.checkLocalLoginPublication(ctx, state.LocalLoginFlowKey, state.AuthSessionID)
}

// PrepareLocalLogin abandons the previous browser flow by atomic generation
// change. A late old response cannot authorize: every read checks the terminal
// flow's exact canonical-session pin. Tombstones remain until normal expiry.
func (s *Service) PrepareLocalLogin(c fiber.Ctx) error {
	if s.localLoginStates == nil {
		return nil
	}
	if err := browsercookie.Check(c, s.store); err != nil {
		return err
	}
	sess, err := s.store.Get(c)
	if err != nil {
		return err
	}
	defer sess.Release()
	if state, ok := sess.Get(sessioncore.AuthAdminKey.String()).(*browserSession); ok && state != nil {
		return goauth.ErrSessionRevoked
	}
	key := "local-login:" + sess.ID()
	r, found, err := s.localLoginStates.Load(c, key)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	owner := r.Owner
	if owner == "" {
		owner = "cancel-local-login:" + uuid.NewString()
		ok, err := s.localLoginStates.Claim(c, key, r.Version, owner)
		if err != nil {
			return err
		}
		if !ok {
			return ErrRefreshInProgress
		}
	}
	r.LocalLoginPending = nil
	r.LocalLoginTerminal = true
	r.LocalLoginSessionID = ""
	ok, err := s.localLoginStates.Complete(c, key, r.Version, owner, r)
	if err != nil {
		return err
	}
	if !ok {
		return ErrRefreshInProgress
	}
	if err := sess.Regenerate(); err != nil {
		return err
	}
	if err := sess.Save(); err != nil {
		return err
	}
	s.writeSessionCookie(c, sess.ID(), false)
	return nil
}

func (s *Service) loadPublishedAdminAuthority(c fiber.Ctx, sess *session.Session, state *browserSession) (bool, error) {
	active, err := s.loadAuthenticatedAuthority(c, sess, state)
	if err != nil || !active {
		return active, err
	}
	err = s.validateLocalLoginPublication(c, state)
	if errors.Is(err, goauth.ErrSessionRevoked) {
		return false, errors.Join(err, s.clearAdminBrowserSession(c, sess))
	}
	return err == nil, err
}

// LocalMFASetupSuggested is a request-local UI hint, never authorization state.
func (s *Service) LocalMFASetupSuggested(c fiber.Ctx) bool {
	suggested, _ := c.Locals(localMFAReminderKey).(bool)
	return suggested
}
