package host

import (
	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/services/adminservice"
)

type (
	ExternalBinding          = adminservice.ExternalBinding
	ExternalProof            = adminservice.ExternalProof
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
