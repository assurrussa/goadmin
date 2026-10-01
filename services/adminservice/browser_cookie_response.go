package adminservice

import "github.com/gofiber/fiber/v3"

// A missing/fenced journal entry says only that this request's ID has no
// authority. The browser may already have accepted a new ID in another response.
// Neither expire the shared cookie name nor re-save this stale Fiber snapshot.
// Drop any cookie queued earlier in this response (e.g. by a session writer),
// while preserving unrelated cookies. Explicit login/rotation may issue a new
// cookie later; canonical revocation and explicit logout are separate operations.
func (s *Service) discardStaleSessionCookies(c fiber.Ctx) {
	c.Response().Header.DelCookie(s.sessionCookieName())
	c.Response().Header.DelCookie(s.policyCookieName())
}
