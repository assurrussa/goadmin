package adminservice

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"

	sessioncore "github.com/assurrussa/goadmin/infrastructure/core/session"
)

// WithCanonicalIdentityLinkReader is installed from the host-owned GoAuth
// store. The RP adapter may use a different database and never reads these
// canonical tables directly.
func WithCanonicalIdentityLinkReader(db *sql.DB, sessions interface {
	IntrospectSession(ctx context.Context, sessionID string) (goauth.SessionSecurity, error)
},
) Option {
	return func(s *Service) error {
		if db == nil || sessions == nil {
			return errors.New("canonical identity reader is required")
		}
		s.identityLinkDB, s.identityLinkSessions = db, sessions
		return nil
	}
}

// IdentityLinkEligibility is read-only guidance, never authority to link.
// LinkExternalIdentity remains the final canonical freshness check.
type IdentityLinkEligibility string

const (
	IdentityLinkFresh    IdentityLinkEligibility = "fresh"
	IdentityLinkStale    IdentityLinkEligibility = "stale"
	IdentityLinkNotLocal IdentityLinkEligibility = "not_local"
)

// WithIdentityLinkAuthMaxAge mirrors the effective GoAuth runtime setting.
// A zero value uses the same ten-minute GoAuth default; invalid values fail closed.
func WithIdentityLinkAuthMaxAge(age time.Duration) Option {
	return func(s *Service) error {
		if age == 0 {
			age = 10 * time.Minute
		}
		if age < time.Minute || age > time.Hour {
			return errors.New("invalid identity link authentication age")
		}
		s.identityLinkAuthMaxAge = age
		return nil
	}
}

func (s *Service) LocalIdentityLinkEligibility(c fiber.Ctx) (IdentityLinkEligibility, error) {
	if s == nil || s.store == nil || s.identityLinkAuthMaxAge < time.Minute {
		return "", goauth.ErrExplicitIdentityLink
	}
	admin, err := s.GetAdminAuth(c)
	if err != nil || admin == nil {
		return "", goauth.ErrSessionRevoked
	}
	sess, err := s.store.Get(c)
	if err != nil {
		return "", err
	}
	defer sess.Release()
	state, ok := sess.Get(sessioncore.AuthAdminKey.String()).(*browserSession)
	if !ok || state == nil || state.SubjectID != admin.AuthSubjectID() || state.AuthSessionID == "" {
		return "", goauth.ErrSessionRevoked
	}
	if state.External != nil {
		return IdentityLinkNotLocal, nil
	}
	if s.identityLinkSessions == nil {
		return "", errors.New("canonical session reader unavailable")
	}
	security, err := s.identityLinkSessions.IntrospectSession(c, state.AuthSessionID)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	if security.Session.SubjectID != state.SubjectID || security.Session.Realm != goauth.RealmAdmin ||
		security.Session.Scope != goauth.SessionScopeAuthenticated || !security.Session.Active(now) {
		return "", goauth.ErrSessionRevoked
	}
	if now.Sub(security.Session.CreatedAt) > s.identityLinkAuthMaxAge {
		return IdentityLinkStale, nil
	}
	return IdentityLinkFresh, nil
}

// HasExternalAdminIdentity reads a canonical GoAuth link for the authenticated
// local subject. An empty external subject asks whether any identity for the
// configured issuer is linked; a nonempty subject checks the exact identity.
func (s *Service) HasExternalAdminIdentity(c fiber.Ctx, issuer, externalSubject string) (bool, error) {
	if s == nil || s.identityLinkDB == nil || issuer == "" {
		return false, errors.New("canonical identity reader unavailable")
	}
	admin, err := s.GetAdminAuth(c)
	if err != nil || admin == nil {
		return false, goauth.ErrSessionRevoked
	}
	var linked bool
	if externalSubject == "" {
		err = s.identityLinkDB.QueryRowContext(c.Context(),
			`SELECT EXISTS (SELECT 1 FROM auth_identity_links WHERE subject_id=$1 AND issuer=$2)`,
			admin.AuthSubjectID(), issuer).Scan(&linked)
	} else {
		err = s.identityLinkDB.QueryRowContext(c.Context(),
			`SELECT EXISTS (SELECT 1 FROM auth_identity_links WHERE subject_id=$1 AND issuer=$2 AND external_subject=$3)`,
			admin.AuthSubjectID(), issuer, externalSubject).Scan(&linked)
	}
	return linked, err
}
