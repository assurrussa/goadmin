package host

import (
	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/bootstrap"
	"github.com/assurrussa/goadmin/services/adminservice"
)

type (
	ExternalBinding          = adminservice.ExternalBinding
	ExternalProof            = adminservice.ExternalProof
	LocalAdminAdmission      = adminservice.LocalAdminAdmission
	ExternalSessionAuthority = adminservice.ExternalSessionAuthority
)

// LinkExternalAdminIdentity explicitly enrolls a verified external identity
// through the current fresh canonical admin session. Host routes own CSRF and
// explicit confirmation. No email auto-link or admin provisioning occurs.
func (a *App) LinkExternalAdminIdentity(c fiber.Ctx, binding ExternalBinding) (goauth.IdentityLink, error) {
	if a == nil || a.inner == nil {
		return goauth.IdentityLink{}, goauth.ErrExplicitIdentityLink
	}
	return a.inner.AdminAuth().LinkExternalAdminIdentity(c, binding)
}

// LoginExternalAdmin creates a canonical admin browser session only for an
// existing confirmed canonical identity link and current local membership.
// The opaque return value is server-only, not a page prop or bearer API token.
func (a *App) LoginExternalAdmin(c fiber.Ctx, binding ExternalBinding) (string, error) {
	if a == nil || a.inner == nil {
		return "", goauth.ErrMembershipDenied
	}
	return a.inner.AdminAuth().LoginExternalAdmin(c, binding)
}

// ExternalAdminBinding reads only protected server metadata; it grants no access.
func (a *App) ExternalAdminBinding(c fiber.Ctx) (*ExternalBinding, error) {
	if a == nil || a.inner == nil {
		return nil, goauth.ErrSessionRevoked
	}
	return a.inner.AdminAuth().ExternalAdminBinding(c)
}

// DetachAdmin clears canonical native authority without provider logout. Hosts
// use it after successful RP-cookie logout or failed admission cleanup.
func (a *App) DetachAdmin(c fiber.Ctx) error {
	if a == nil || a.inner == nil {
		return goauth.ErrSessionRevoked
	}
	return a.inner.AdminAuth().DetachAdminAuth(c)
}

// LogoutAdmin is explicit user intent. Host routes MUST enforce authentication,
// CSRF and origin BEFORE calling this; it must precede provider Verify/Check.
func (a *App) LogoutAdmin(c fiber.Ctx) error {
	if a == nil || a.inner == nil {
		return goauth.ErrSessionRevoked
	}
	return a.inner.AdminAuth().DelAdminAuth(c)
}

// CSRFSessionBinding uses the same opaque browser binding as core admin CSRF.
func CSRFSessionBinding(cookie string) string { return bootstrap.CSRFSessionBinding(cookie) }

// AdminCSRFRequest returns the current canonical admin browser binding for a
// host-owned form. It performs native authentication; metadata alone is not proof.
func (a *App) AdminCSRFRequest(c fiber.Ctx) (CSRFRequest, error) {
	if a == nil || a.inner == nil {
		return CSRFRequest{}, goauth.ErrSessionRevoked
	}
	admin, err := a.inner.AdminAuth().GetAdminAuth(c)
	if err != nil {
		return CSRFRequest{}, err
	}
	if admin == nil {
		return CSRFRequest{}, goauth.ErrSessionRevoked
	}
	return CSRFRequest{SessionID: CSRFSessionBinding(admin.SessionID), UserID: admin.UUID}, nil
}
