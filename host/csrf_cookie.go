package host

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/bootstrap"
	adminconfig "github.com/assurrussa/goadmin/config"
)

// WriteAdminCSRFCookie uses the same scope and lifetime policy as the native
// admin bootstrap. It is for already-issued CSRF proofs, not authentication.
func (a *App) WriteAdminCSRFCookie(c fiber.Ctx, name, token string, ttl time.Duration) error {
	if a == nil || a.inner == nil || a.inner.Session() == nil || c == nil || name == "" || token == "" || ttl <= 0 {
		return errors.New("admin CSRF cookie policy is unavailable")
	}
	bootstrap.WriteAdminCSRFCookie(c, a.inner.Session(), adminconfig.Config{
		CSRFTokenName: name,
		CSRFTokenTTL:  ttl,
	}, token, "")
	return nil
}
