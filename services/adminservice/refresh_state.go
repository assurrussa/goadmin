package adminservice

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/google/uuid"

	"github.com/assurrussa/goadmin/internal/auth/browserstate"
	"github.com/assurrussa/goadmin/internal/refreshpolicy"
)

var (
	ErrRefreshInProgress = errors.New("admin refresh is in progress; retry later")
	// ErrRefreshRetryRequired preserves browser authority when an access token
	// still needs refresh after a completed, definitely unconsumed attempt.
	ErrRefreshRetryRequired = errors.New("admin access token requires another refresh attempt")
)

func WithBrowserState(store browserstate.Store) Option {
	return func(s *Service) error {
		if store == nil {
			return errors.New("atomic admin browser state is required")
		}
		s.states = browserstate.Guarded(store)
		return nil
	}
}

func applyBrowserRecord(state *browserSession, r browserstate.Record) {
	state.Version = r.Version
	state.SubjectID = r.SubjectID
	state.AccessToken = r.Tokens.AccessToken
	state.RefreshToken = r.Tokens.RefreshToken
	state.AuthSessionID = r.Tokens.Session.ID
}

func (s *Service) refreshOwnedAdminToken(c fiber.Ctx, sess *session.Session, state *browserSession) (*goauth.AuthContext, error) {
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	defer cancel()
	owner := refreshpolicy.Owner("refresh", uuid.NewString(), time.Now())
	claimed, err := s.states.Claim(ctx, sess.ID(), state.Version, owner)
	if err != nil {
		return nil, s.releaseBeforeConsumption(c, sess, state.Version, owner, fmt.Errorf("claim admin refresh: %w", err))
	}
	if !claimed {
		return s.waitAdminRefresh(c, sess, state)
	}
	old, found, err := s.states.Load(ctx, sess.ID())
	if err != nil {
		return nil, s.releaseBeforeConsumption(c, sess, state.Version, owner, fmt.Errorf("read claimed admin refresh: %w", err))
	}
	if !found || old.Owner != owner || old.Version != state.Version {
		s.discardStaleSessionCookies(c)
		return nil, goauth.ErrOperationOutcomeUnknown
	}
	rotated, refreshErr := s.runtime.Refresh(ctx, old.Tokens.RefreshToken)
	if refreshErr != nil {
		return s.failOwnedAdminRefresh(c, sess, old, owner, refreshErr)
	}
	old.Tokens = rotated
	// Persist a known rotation even when the incoming request has been canceled.
	// This does not retry Refresh; failure leaves the claim unresolved.
	commitCtx, stop := context.WithTimeout(context.WithoutCancel(c.Context()), 2*time.Second)
	defer stop()
	ok, err := s.states.Complete(commitCtx, sess.ID(), old.Version, owner, old)
	if err != nil || !ok {
		s.deleteSessionCookies(c)
		return nil, errors.Join(goauth.ErrOperationOutcomeUnknown, err)
	}
	old.Version++
	old.Owner = ""
	applyBrowserRecord(state, old)
	return s.verifyCommittedRefresh(c, sess, state)
}

// Release is safe ONLY before Runtime.Refresh has been called. An ambiguous
// claim write is also safe to release here because this owner has not consumed
// anything. The compare-and-swap prevents releasing another owner's claim.
func (s *Service) releaseBeforeConsumption(c fiber.Ctx, sess *session.Session, version int64, owner string, cause error) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Context()), time.Second)
	defer cancel()
	_, err := s.states.ReleaseClaim(ctx, sess.ID(), version, owner)
	if err != nil {
		s.deleteSessionCookies(c)
		return errors.Join(goauth.ErrOperationOutcomeUnknown, cause, err)
	}
	return cause
}

func (s *Service) verifyCommittedRefresh(c fiber.Ctx, sess *session.Session, state *browserSession) (*goauth.AuthContext, error) {
	auth, err := s.runtime.VerifyAccessToken(c, state.AccessToken, true)
	if err == nil {
		return &auth, nil
	}
	if isTerminalSessionError(err) {
		if clearErr := s.clearAdminBrowserSession(c, sess); clearErr != nil {
			return nil, clearErr
		}
		return nil, nil //nolint:nilnil // terminal canonical failure is anonymous
	}
	if errors.Is(err, goauth.ErrExpiredToken) || errors.Is(err, goauth.ErrInvalidToken) {
		// A new journal version may acknowledge a deterministic failed refresh,
		// keeping the old pair. Access expiry is NOT evidence that the refresh
		// credential is invalid. Keep authority and let a later request acquire
		// its own CAS claim; never recurse or replay the current mutation here.
		return nil, ErrRefreshRetryRequired
	}
	return nil, fmt.Errorf("verify committed admin refresh: %w", err)
}

func (s *Service) rotateOwnedBrowserID(c fiber.Ctx, sess *session.Session, state *browserSession) error {
	oldID := sess.ID()
	r, found, err := s.states.Load(c, oldID)
	if err != nil {
		return err
	}
	if !found {
		return goauth.ErrSessionRevoked
	}
	owner := refreshpolicy.Owner("rotate", uuid.NewString(), time.Now())
	claimed, err := s.states.Claim(c, oldID, r.Version, owner)
	if err != nil {
		return err
	}
	if !claimed {
		return ErrRefreshInProgress
	}
	// Readers must reject rotate claims, not just refreshers. A failed move
	// never restores authority to the old opaque browser identifier.
	if err := sess.Regenerate(); err != nil {
		s.deleteSessionCookies(c)
		return errors.Join(goauth.ErrOperationOutcomeUnknown, err)
	}
	if err := s.states.Create(c, sess.ID(), r, min(s.sessionTTL, time.Until(r.Tokens.Session.ExpiresAt))); err != nil {
		s.deleteSessionCookies(c)
		return errors.Join(goauth.ErrOperationOutcomeUnknown, err)
	}
	if err := s.states.Delete(c, oldID); err != nil {
		_ = s.states.Delete(c, sess.ID())
		s.deleteSessionCookies(c)
		return errors.Join(goauth.ErrOperationOutcomeUnknown, err)
	}
	r.Version = 1
	r.Owner = ""
	applyBrowserRecord(state, r)
	state.AccessToken = ""
	state.RefreshToken = ""
	return nil
}

func (s *Service) waitAdminRefresh(c fiber.Ctx, sess *session.Session, state *browserSession) (*goauth.AuthContext, error) {
	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()
	for attempt := 0; ; attempt++ {
		r, found, err := s.states.Load(ctx, sess.ID())
		if err != nil {
			if ctx.Err() != nil {
				return nil, ErrRefreshInProgress
			}
			return nil, fmt.Errorf("read concurrent admin refresh: %w", err)
		}
		if !found {
			s.discardStaleSessionCookies(c)
			return nil, nil //nolint:nilnil // session was cleared
		}
		if refreshpolicy.RequiresLogin(r.Owner, time.Now()) {
			s.discardStaleSessionCookies(c)
			return nil, goauth.ErrOperationOutcomeUnknown
		}
		if r.Owner == "" {
			if r.Version == state.Version {
				return nil, ErrRefreshInProgress // safe pre-consumption rollback
			}
			applyBrowserRecord(state, r)
			return s.verifyCommittedRefresh(c, sess, state)
		}
		delay := refreshpolicy.Backoff(attempt)
		delay = delay*3/4 + time.Duration(rand.Int64N(int64(delay/4)+1)) //nolint:gosec // bounded jitter, not cryptographic randomness
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			// A slow live owner is not an unknown outcome. Do not erase a cookie
			// that a successful concurrent response is still using.
			return nil, ErrRefreshInProgress
		}
	}
}

func (s *Service) failOwnedAdminRefresh(
	c fiber.Ctx, sess *session.Session, old browserstate.Record, owner string, refreshErr error,
) (*goauth.AuthContext, error) {
	if errors.Is(refreshErr, goauth.ErrOperationOutcomeUnknown) {
		s.deleteSessionCookies(c)
		return nil, refreshErr
	}
	if isTerminalSessionError(refreshErr) || isInvalidRefreshToken(refreshErr) {
		if err := s.clearAdminBrowserSession(c, sess); err != nil {
			return nil, err
		}
		return nil, nil //nolint:nilnil // terminal canonical failure
	}
	// The canonical Runtime contract classifies ambiguous consumption explicitly.
	// Deterministic errors can restore the old tokens with a new journal version.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Context()), 2*time.Second)
	defer cancel()
	ok, err := s.states.Complete(ctx, sess.ID(), old.Version, owner, old)
	if err != nil || !ok {
		s.deleteSessionCookies(c)
		return nil, errors.Join(goauth.ErrOperationOutcomeUnknown, err)
	}
	return nil, fmt.Errorf("refresh admin token: %w", refreshErr)
}
