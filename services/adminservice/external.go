package adminservice

import (
	"context"
	"errors"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"

	sessioncore "github.com/assurrussa/goadmin/infrastructure/core/session"
	"github.com/assurrussa/goadmin/internal/auth/browsercookie"
)

const externalFreshnessCap = 5 * time.Minute

// ExternalProof is verified provider time, never the time a response was received.
// Validity confirms the admitted immutable snapshot. Fresh provider proof needs
// explicit admission; neither issue time nor deadline slides on a read.
type ExternalProof struct{ IssuedAt, ExpiresAt time.Time }

// ExternalBinding is server-side metadata, never a browser credential. LinkID
// pins the canonical link INSTANCE so unlink/relink cannot revive old sessions.
// AuthorityID references the host's encrypted durable provider-state journal.
type ExternalBinding struct {
	Issuer, Subject, LinkID, AuthorityID, ProviderSessionID string
	ExternalAuthTime                                        time.Time
	Generation                                              uint64
	LoginGeneration                                         uint64
	Deadline                                                time.Time
	LocalSubject                                            goauth.SubjectID
	AbsoluteUntil                                           time.Time
	Proof                                                   ExternalProof
}

// ExternalSessionAuthority is neutral: goadmin does not depend on an OIDC/RP
// client. Validate checks exact binding, grant and fresh provider authority; its
// owner/CAS journal, refresh and revocation belong to the reusable RP component.
// Validate MUST be a read-only snapshot check: no network or refresh. A new
// generation/proof requires explicit admission; the saved deadline is immutable.
// No network request may be made while holding a DB transaction/lock.
// Detach cleans up a superseded native admission only; it MUST NOT tombstone or
// revoke RP authority, including the same or a newer generation.
// Logout is explicit user intent: durably terminate the pinned RP session across
// all generations and fence in-flight renewal before bounded best-effort revoke.
// A stale generation MUST NOT make explicit logout a no-op. Validate the immutable
// reference/identity pins and immutable LoginGeneration independently of refresh
// generation, never target another callback login even with identical claims.
// Neither operation may retry a consumed refresh secret.
type ExternalSessionAuthority interface {
	Validate(ctx context.Context, binding ExternalBinding) (ExternalProof, error)
	Detach(ctx context.Context, binding ExternalBinding) error
	Logout(ctx context.Context, binding ExternalBinding) error
}

type IdentityLinkResolver interface {
	ResolveIdentityLink(ctx context.Context, issuer, subject string) (goauth.Account, goauth.IdentityLink, error)
}

type externalRuntime interface {
	LoginExternal(ctx context.Context, request goauth.ExternalLoginRequest) (goauth.LoginResult, error)
	LinkExternalIdentity(ctx context.Context, auth goauth.AuthContext, identity goauth.ExternalIdentity) (goauth.IdentityLink, error)
}

// WithExternalAuthority is opt-in. Missing authority on an already external
// session always denies; it must never turn that session into a password session.
func WithExternalAuthority(authority ExternalSessionAuthority, links IdentityLinkResolver) Option {
	return func(s *Service) error {
		if authority != nil && links == nil {
			return errors.New("external session canonical link resolver required")
		}
		s.externalAuthority = authority
		s.externalLinks = links
		return nil
	}
}

// The host must construct bindings from verified opaque RP snapshots, never
// decode untrusted browser JSON into ExternalBinding.
//
// LinkExternalAdminIdentity requires the CURRENT canonical authenticated admin
// session. GoAuth itself enforces the session's freshness for explicit linking.
// Host routes must also require CSRF, explicit confirmation and verified provider
// proof; neither provider JSON nor a matching email is an enrollment instruction.
// No email/profile is forwarded, and no account/membership is provisioned.
func (s *Service) LinkExternalAdminIdentity(c fiber.Ctx, binding ExternalBinding) (goauth.IdentityLink, error) {
	rt, ok := s.runtime.(externalRuntime)
	if c.Method() != fiber.MethodPost || !ok || s.externalAuthority == nil {
		return goauth.IdentityLink{}, goauth.ErrExplicitIdentityLink
	}
	admin, err := s.GetAdminAuth(c)
	if err != nil {
		return goauth.IdentityLink{}, err
	}
	if admin == nil || admin.SubjectID != binding.LocalSubject {
		return goauth.IdentityLink{}, goauth.ErrExplicitIdentityLink
	}
	if _, err := s.externalProof(c, binding); err != nil {
		return goauth.IdentityLink{}, err
	}
	sess, err := s.store.Get(c)
	if err != nil {
		return goauth.IdentityLink{}, err
	}
	defer sess.Release()
	state, ok := sess.Get(sessioncore.AuthAdminKey.String()).(*browserSession)
	// Use the same private key as the existing canonical auth implementation.
	if !ok || state == nil {
		return goauth.IdentityLink{}, goauth.ErrExplicitIdentityLink
	}
	if active, err := s.loadBrowserAuthority(c, sess, state); err != nil || !active {
		if err != nil {
			return goauth.IdentityLink{}, err
		}
		return goauth.IdentityLink{}, goauth.ErrExplicitIdentityLink
	}
	auth, err := s.runtime.VerifyAccessToken(c, state.AccessToken, true)
	if err != nil || auth.SubjectID != binding.LocalSubject || auth.Realm != goauth.RealmAdmin {
		return goauth.IdentityLink{}, goauth.ErrExplicitIdentityLink
	}
	return rt.LinkExternalIdentity(c, auth, goauth.ExternalIdentity{Issuer: binding.Issuer, Subject: binding.Subject})
}

// LoginExternalAdmin accepts only a verified authority and EXISTING canonical
// link to the expected local subject. GoAuth receives no email/profile, making
// its missing-link path unable to auto-link or provision. Local membership,
// journal encryption, identifier regeneration and RBAC use the existing path.
// The returned opaque ID is for server-side adapter binding, not page props.
func (s *Service) LoginExternalAdmin(c fiber.Ctx, binding ExternalBinding) (string, error) {
	rt, ok := s.runtime.(externalRuntime)
	if !ok || s.externalAuthority == nil || s.externalLinks == nil {
		return "", goauth.ErrMembershipDenied
	}
	if err := browsercookie.Check(c, s.store); err != nil {
		return "", err
	}
	link, err := s.canonicalExternalLink(c, binding)
	if err != nil {
		return "", err
	}
	if link.ID != binding.LinkID {
		return "", goauth.ErrExplicitIdentityLink
	}
	proof, err := s.externalProof(c, binding)
	if err != nil {
		return "", err
	}
	binding.Proof = proof
	result, err := rt.LoginExternal(c, goauth.ExternalLoginRequest{
		Realm:    goauth.RealmAdmin,
		Identity: goauth.ExternalIdentity{Issuer: binding.Issuer, Subject: binding.Subject},
	})
	if err != nil {
		return "", err
	}
	revoke := func() {
		_ = s.runtime.Logout(context.WithoutCancel(c.Context()), result.Account.Subject.ID, result.Tokens.Session.ID)
	}
	if result.Account.Subject.ID != binding.LocalSubject {
		revoke()
		return "", goauth.ErrExplicitIdentityLink
	}
	admin, err := s.admins.GetBySubjectID(c, binding.LocalSubject)
	if err != nil || admin.ID <= 0 {
		revoke()
		return "", goauth.ErrMembershipDenied
	}
	if current, err := s.canonicalExternalLink(c, binding); err != nil || current.ID != link.ID {
		revoke()
		return "", goauth.ErrExplicitIdentityLink
	}
	// Evict the old native journal/session before writing a newly admitted one.
	if err := s.delAdminAuth(c, false); err != nil {
		revoke()
		return "", err
	}
	if _, err := s.externalProof(c, binding); err != nil {
		revoke()
		return "", err
	}
	if current, err := s.canonicalExternalLink(c, binding); err != nil || current.ID != link.ID {
		revoke()
		return "", goauth.ErrExplicitIdentityLink
	}
	if err := s.saveAuthenticatedSession(c, admin, result.Account, result.Tokens, false, &binding); err != nil {
		revoke()
		return "", err
	}
	sess, err := s.store.Get(c)
	if err != nil {
		revoke()
		return "", err
	}
	defer sess.Release()
	return sess.ID(), nil
}

func (s *Service) canonicalExternalLink(ctx context.Context, b ExternalBinding) (goauth.IdentityLink, error) {
	if s.externalLinks == nil || b.Issuer == "" || b.Subject == "" || b.LocalSubject.IsZero() {
		return goauth.IdentityLink{}, goauth.ErrExplicitIdentityLink
	}
	account, link, err := s.externalLinks.ResolveIdentityLink(ctx, b.Issuer, b.Subject)
	if err != nil {
		return goauth.IdentityLink{}, err
	}
	if account.Subject.ID != b.LocalSubject || account.Subject.Status != goauth.SubjectStatusActive ||
		link.SubjectID != b.LocalSubject || link.ID == "" || link.Issuer != b.Issuer || link.ExternalSubject != b.Subject {
		return goauth.IdentityLink{}, goauth.ErrExplicitIdentityLink
	}
	return link, nil
}

func (s *Service) externalProof(ctx context.Context, b ExternalBinding) (ExternalProof, error) {
	if s.externalAuthority == nil || b.LoginGeneration == 0 || b.Generation == 0 || b.Deadline.IsZero() || b.AuthorityID == "" ||
		b.ProviderSessionID == "" || b.ExternalAuthTime.IsZero() || b.ExternalAuthTime.After(b.Proof.IssuedAt) ||
		b.LocalSubject.IsZero() || b.Issuer == "" || b.Subject == "" ||
		!time.Now().Before(b.AbsoluteUntil) || !time.Now().Before(b.Deadline) {
		return ExternalProof{}, goauth.ErrSessionRevoked
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	proof, err := s.externalAuthority.Validate(ctx, b)
	if err != nil {
		return ExternalProof{}, err
	}
	deadline := externalDeadline(proof, b.AbsoluteUntil)
	if !proof.IssuedAt.Equal(b.Proof.IssuedAt) || !proof.ExpiresAt.Equal(b.Proof.ExpiresAt) || b.Deadline.After(deadline) {
		return ExternalProof{}, goauth.ErrSessionRevoked
	}
	deadline = b.Deadline
	now := time.Now()
	if proof.IssuedAt.IsZero() || proof.IssuedAt.After(now) || !proof.ExpiresAt.After(proof.IssuedAt) ||
		!now.Before(deadline) || proof.IssuedAt.Before(b.Proof.IssuedAt) {
		return ExternalProof{}, goauth.ErrSessionRevoked
	}
	if proof.IssuedAt.Equal(b.Proof.IssuedAt) && deadline.After(externalDeadline(b.Proof, b.AbsoluteUntil)) {
		return ExternalProof{}, goauth.ErrSessionRevoked
	}
	return proof, nil
}

func (s *Service) checkExternal(c fiber.Ctx, sess *session.Session, state *browserSession) error {
	if state.External == nil {
		return nil
	}
	binding := *state.External
	link, err := s.canonicalExternalLink(c, binding)
	if err == nil && link.ID != binding.LinkID {
		err = goauth.ErrExplicitIdentityLink
	}
	if err == nil {
		var proof ExternalProof
		proof, err = s.externalProof(c, binding)
		if err == nil {
			state.External.Proof = proof
			return nil
		}
	}
	// Clearing native authority never clears only the external marker. Disable,
	// unlink/relink, missing binding and dependency failure remain fail-closed.
	if errors.Is(err, goauth.ErrIdentityLinkNotFound) || errors.Is(err, goauth.ErrExplicitIdentityLink) {
		err = goauth.ErrSessionRevoked
	}
	clearErr := s.clearAdminBrowserSession(c, sess)
	return errors.Join(err, clearErr)
}

func externalDeadline(p ExternalProof, absolute time.Time) time.Time {
	end := p.IssuedAt.Add(externalFreshnessCap)
	if p.ExpiresAt.Before(end) {
		end = p.ExpiresAt
	}
	if absolute.Before(end) {
		end = absolute
	}
	return end
}

func (s *Service) finishExternal(ctx context.Context, b *ExternalBinding, userLogout bool) error {
	if b == nil || s.externalAuthority == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if userLogout {
		return s.externalAuthority.Logout(ctx, *b)
	}
	return s.externalAuthority.Detach(ctx, *b)
}

// Keep native journal ownership checks and external validity in one admission
// checkpoint, before projection building or any protected handler executes.
func (s *Service) loadAuthenticatedAuthority(c fiber.Ctx, sess *session.Session, state *browserSession) (bool, error) {
	active, err := s.loadBrowserAuthority(c, sess, state)
	if err != nil || !active {
		return active, err
	}
	if err := s.checkExternal(c, sess, state); err != nil {
		return false, err
	}
	return true, nil
}

// A native refresh/ACL read may span the proof deadline. Check it again before
// persisting/returning the projection; never give a request a receipt-time grace.
func (s *Service) saveAuthenticatedProjection(c fiber.Ctx, sess *session.Session, state *browserSession) error {
	if state.External != nil {
		if !time.Now().Before(state.External.Deadline) {
			return errors.Join(goauth.ErrSessionRevoked, s.clearAdminBrowserSession(c, sess))
		}
		state.Payload.ExternalValidUntil = state.External.Deadline
	}
	return sess.Save()
}
