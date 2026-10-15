package adminservice

import (
	"context"
	"errors"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/google/uuid"

	sessioncore "github.com/assurrussa/goadmin/infrastructure/core/session"
	"github.com/assurrussa/goadmin/internal/auth/browsercookie"
	"github.com/assurrussa/goadmin/internal/auth/browserstate"
	"github.com/assurrussa/goadmin/internal/refreshpolicy"
)

// ExternalAdminRecovery is an opaque, short-lived server-only snapshot. It is
// not authentication, cannot authorize commands, and must never enter page
// props, cookies, request bodies or host persistence.
type ExternalAdminRecovery struct {
	service *Service
	id      string
	record  browserstate.Record
	binding ExternalBinding
	until   time.Time
}

// Binding returns a copy of identity/lineage pins for the host's SDK verifier.
func (r *ExternalAdminRecovery) Binding() ExternalBinding {
	if r == nil {
		return ExternalBinding{}
	}
	return r.binding
}

var ErrExternalRecoveryRequired = errors.New("external proof requires explicit recovery")

func recoverableExternalDeadline(b ExternalBinding, now time.Time) bool {
	return !b.Deadline.IsZero() && !now.Before(b.Deadline) &&
		now.Before(b.AbsoluteUntil) && b.Generation != 0 && b.LoginGeneration != 0 &&
		b.AuthorityID != "" && b.ProviderSessionID != "" && b.LinkID != "" &&
		b.Issuer != "" && b.Subject != "" && !b.LocalSubject.IsZero() &&
		!b.ExternalAuthTime.IsZero() && !b.Proof.IssuedAt.IsZero() &&
		!b.ExternalAuthTime.After(b.Proof.IssuedAt) &&
		b.Proof.ExpiresAt.After(b.Proof.IssuedAt) &&
		!b.Deadline.After(externalDeadline(b.Proof, b.AbsoluteUntil))
}

// BeginExternalAdminRecovery must precede the host's remote SDK verification.
// Hosts must guard their dedicated endpoint with same-origin and explicit
// recovery-header checks. This method neither refreshes nor authenticates.
// Legacy journals without an external binding require a new ordinary login.
func (s *Service) BeginExternalAdminRecovery(c fiber.Ctx) (*ExternalAdminRecovery, error) {
	if s == nil || s.states == nil || s.runtime == nil || s.externalAuthority == nil || s.externalLinks == nil {
		return nil, goauth.ErrSessionRevoked
	}
	if err := browsercookie.Check(c, s.store); err != nil {
		return nil, err
	}
	sess, err := s.store.Get(c)
	if err != nil {
		return nil, err
	}
	defer sess.Release()
	record, found, err := s.states.Load(c, sess.ID())
	if err != nil {
		return nil, err
	}
	if !found || record.Owner != "" || record.External == nil || record.SubjectID != record.External.LocalSubject ||
		record.Tokens.Session.ID == "" || !time.Now().Before(record.Tokens.Session.ExpiresAt) ||
		!time.Now().Before(record.External.AbsoluteUntil) {
		return nil, goauth.ErrSessionRevoked
	}
	binding := *record.External
	link, err := s.canonicalExternalLink(c, binding)
	if err != nil {
		return nil, err
	}
	if link.ID != binding.LinkID {
		return nil, goauth.ErrExplicitIdentityLink
	}
	admin, err := s.admins.GetBySubjectID(c, binding.LocalSubject)
	if err != nil {
		return nil, err
	}
	if admin.ID <= 0 {
		return nil, goauth.ErrMembershipDenied
	}
	return &ExternalAdminRecovery{
		service: s, id: sess.ID(), record: record, binding: binding, until: time.Now().Add(20 * time.Second),
	}, nil
}

func sameExternalLogin(old, fresh ExternalBinding) bool {
	if fresh.Generation == old.Generation &&
		(!fresh.Deadline.Equal(old.Deadline) || !fresh.AbsoluteUntil.Equal(old.AbsoluteUntil) ||
			!fresh.Proof.IssuedAt.Equal(old.Proof.IssuedAt) || !fresh.Proof.ExpiresAt.Equal(old.Proof.ExpiresAt)) {
		return false
	}
	return old.Issuer == fresh.Issuer && old.Subject == fresh.Subject &&
		old.LinkID == fresh.LinkID && old.LocalSubject == fresh.LocalSubject &&
		old.AuthorityID == fresh.AuthorityID && old.ProviderSessionID == fresh.ProviderSessionID &&
		old.ClientID == fresh.ClientID && old.ProjectID == fresh.ProjectID &&
		old.ExternalAuthTime.Equal(fresh.ExternalAuthTime) && old.LoginGeneration == fresh.LoginGeneration &&
		fresh.Generation >= old.Generation && !fresh.Proof.IssuedAt.Before(old.Proof.IssuedAt) &&
		!fresh.AbsoluteUntil.After(old.AbsoluteUntil)
}

// CompleteExternalAdminRecovery admits only fresh verified SDK evidence for the
// captured login. The existing native session/cookie and absolute expiry remain;
// no new canonical login is created. Journal CAS fences concurrent logout,
// rotation and refresh. A canonical revocation cannot be resurrected because
// the canonical session ID never changes. No remote call occurs under a DB lock.
func (s *Service) CompleteExternalAdminRecovery(c fiber.Ctx, recovery *ExternalAdminRecovery, fresh ExternalBinding) error {
	if c.Method() != fiber.MethodGet || recovery == nil || recovery.service != s || !time.Now().Before(recovery.until) ||
		!time.Now().Before(recovery.binding.AbsoluteUntil) || !sameExternalLogin(recovery.binding, fresh) {
		return goauth.ErrSessionRevoked
	}
	if err := browsercookie.Check(c, s.store); err != nil {
		return err
	}
	sess, err := s.store.Get(c)
	if err != nil {
		return err
	}
	defer sess.Release()
	if sess.ID() != recovery.id {
		return goauth.ErrSessionRevoked
	}
	if err := s.checkExternalRecoveryAdmission(c, fresh, recovery.binding.LinkID); err != nil {
		return err
	}
	owner := refreshpolicy.Owner("refresh", uuid.NewString(), time.Now())
	claimed, err := s.states.Claim(c, recovery.id, recovery.record.Version, owner)
	if err != nil || !claimed {
		return errors.Join(goauth.ErrSessionRevoked, err)
	}
	// Before canonical refresh consumes anything this claim can be released.
	consumed := false
	defer func() {
		if !consumed {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Context()), time.Second)
			defer cancel()
			_, _ = s.states.ReleaseClaim(ctx, recovery.id, recovery.record.Version, owner)
		}
	}()
	record, found, err := s.states.Load(c, recovery.id)
	if err != nil || !found || record.External == nil || record.Version != recovery.record.Version || record.Owner != owner ||
		record.SubjectID != recovery.record.SubjectID || record.Tokens.Session.ID != recovery.record.Tokens.Session.ID {
		return errors.Join(goauth.ErrSessionRevoked, err)
	}
	auth, err := s.verifyExternalRecoveryToken(c, sess, &record, owner, &consumed)
	if err != nil {
		return err
	}
	if auth.SubjectID != fresh.LocalSubject || auth.SessionID != record.Tokens.Session.ID || auth.Realm != goauth.RealmAdmin {
		return goauth.ErrSessionRevoked
	}
	// Recheck local and provider authority after any canonical refresh.
	if _, err := s.externalProof(c, fresh); err != nil {
		return err
	}
	link, err := s.canonicalExternalLink(c, fresh)
	if err != nil || link.ID != fresh.LinkID {
		return errors.Join(goauth.ErrExplicitIdentityLink, err)
	}
	admin, err := s.admins.GetBySubjectID(c, fresh.LocalSubject)
	if err != nil || admin.ID <= 0 {
		return errors.Join(goauth.ErrMembershipDenied, err)
	}
	if !time.Now().Before(recovery.until) || !time.Now().Before(fresh.Deadline) || !time.Now().Before(fresh.AbsoluteUntil) {
		return goauth.ErrSessionRevoked
	}
	binding := fresh
	record.External = &binding
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Context()), 2*time.Second)
	defer cancel()
	committed, err := s.states.Complete(ctx, recovery.id, recovery.record.Version, owner, record)
	if err != nil || !committed {
		return errors.Join(goauth.ErrOperationOutcomeUnknown, err)
	}
	// Journal is authoritative; stale parallel Fiber saves cannot undo this.
	// Keep only the projection in Fiber, never raw canonical credentials.
	state, ok := sess.Get(sessioncore.AuthAdminKey.String()).(*browserSession)
	if !ok || state == nil {
		return goauth.ErrSessionRevoked
	}
	record.Version++
	record.Owner = ""
	applyBrowserRecord(state, record)
	state.AccessToken, state.RefreshToken = "", ""
	state.Payload.ExternalValidUntil = fresh.Deadline
	sess.Set(sessioncore.AuthAdminKey.String(), state)
	return sessioncore.SavePassive(c, s.store, sess)
}

func (s *Service) checkExternalRecoveryAdmission(c fiber.Ctx, fresh ExternalBinding, linkID string) error {
	if _, err := s.externalProof(c, fresh); err != nil {
		return err
	}
	link, err := s.canonicalExternalLink(c, fresh)
	if err != nil {
		return err
	}
	if link.ID != linkID {
		return goauth.ErrExplicitIdentityLink
	}
	admin, err := s.admins.GetBySubjectID(c, fresh.LocalSubject)
	if err != nil {
		return err
	}
	if admin.ID <= 0 {
		return goauth.ErrMembershipDenied
	}
	return nil
}

func (s *Service) verifyExternalRecoveryToken(
	c fiber.Ctx, sess *session.Session, record *browserstate.Record, owner string, consumed *bool,
) (goauth.AuthContext, error) {
	auth, err := s.runtime.VerifyAccessToken(c, record.Tokens.AccessToken, true)
	// GoAuth also reports JWT expiry as ErrInvalidToken. Only the trusted
	// journal expiry can admit that case. Invalid alone is never sufficient;
	// the canonical refresh credential must still prove current authority.
	expired := !record.Tokens.AccessExpiresAt.IsZero() && !time.Now().Before(record.Tokens.AccessExpiresAt)
	if errors.Is(err, goauth.ErrExpiredToken) || (errors.Is(err, goauth.ErrInvalidToken) && expired) {
		*consumed = true
		tokens, refreshErr := s.runtime.Refresh(c, record.Tokens.RefreshToken)
		if refreshErr != nil {
			_, handledErr := s.failOwnedAdminRefresh(c, sess, *record, owner, refreshErr)
			return goauth.AuthContext{}, errors.Join(goauth.ErrSessionRevoked, handledErr)
		}
		// Never transform recovery into a new canonical login.
		if tokens.Session.ID != record.Tokens.Session.ID || tokens.Session.ExpiresAt.After(record.Tokens.Session.ExpiresAt) {
			return goauth.AuthContext{}, goauth.ErrSessionRevoked
		}
		record.Tokens = tokens
		auth, err = s.runtime.VerifyAccessToken(c, tokens.AccessToken, true)
	}
	return auth, err
}

type externalRecoveryEnabledKey struct{}

// EnableExternalAdminRecovery is called only by trusted host middleware that
// registered a guarded recovery endpoint. Browser headers cannot enable it.
func EnableExternalAdminRecovery(c fiber.Ctx) {
	c.Locals(externalRecoveryEnabledKey{}, true)
}

func ExternalAdminRecoveryEnabled(c fiber.Ctx) bool {
	enabled, _ := c.Locals(externalRecoveryEnabledKey{}).(bool)
	return enabled
}
