package host

import (
	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/services/adminservice"
)

type ExternalAdminRecovery = adminservice.ExternalAdminRecovery

var ErrExternalRecoveryRequired = adminservice.ErrExternalRecoveryRequired

// BeginExternalAdminRecovery captures an opaque native fence before SDK Verify.
// It is never an authentication result. Hosts must guard the dedicated endpoint
// against cross-origin requests and must not expose the snapshot to the browser.
func (a *App) BeginExternalAdminRecovery(c fiber.Ctx) (*ExternalAdminRecovery, error) {
	if a == nil || a.inner == nil {
		return nil, goauth.ErrSessionRevoked
	}
	return a.inner.AdminAuth().BeginExternalAdminRecovery(c)
}

// CompleteExternalAdminRecovery replaces verified provider evidence in place.
// It never creates a canonical login, rotates the cookie or extends the signed
// absolute lifetime. Obtain AdminCSRFRequest afterwards before reporting success.
func (a *App) CompleteExternalAdminRecovery(c fiber.Ctx, recovery *ExternalAdminRecovery, fresh ExternalBinding) error {
	if a == nil || a.inner == nil {
		return goauth.ErrSessionRevoked
	}
	return a.inner.AdminAuth().CompleteExternalAdminRecovery(c, recovery, fresh)
}

// EnableExternalAdminRecovery opts this request into the recovery response
// marker. Call only from middleware of a host that registered the guarded
// recovery endpoint; never derive this opt-in from a browser request header.
func EnableExternalAdminRecovery(c fiber.Ctx) {
	adminservice.EnableExternalAdminRecovery(c)
}

// EnableExternalAdminRecoveryWithSignIn also supplies a fixed host-owned public
// sign-in continuation. It must bypass native auth and require ordinary verified
// login/admission. Never pass a browser-selected URL or a protected login loop.
// Invalid or native /auth/login destinations are not rendered.
func EnableExternalAdminRecoveryWithSignIn(c fiber.Ctx, target string) {
	EnableExternalAdminRecovery(c)
	adminservice.SetExternalAdminRecoverySignIn(c, target)
}
