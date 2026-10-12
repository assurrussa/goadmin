package session

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
)

// SavePassive persists projection/flash/CSRF state without renewing an existing
// browser credential. A completed response may arrive after logout and another
// login; echoing its request cookie would then replace the newer credential.
// Explicit cookies already queued in this response (login, rotation, logout)
// are retained byte-for-byte, including their expiry and scope. Fresh anonymous
// bootstrap may issue a cookie, including replacement of an expired/unknown
// request ID; otherwise login CSRF would remain bound to an unreachable session.
// Storage idle TTL is still refreshed; browser persistence/expiry remains the
// policy established by explicit authentication, never a passive sliding lease.
func SavePassive(c fiber.Ctx, store *session.Store, sess *session.Session) error {
	name := store.Extractor.Key
	if name == "" {
		name = "session_id"
	}
	var previous [][]byte
	for key, value := range c.Response().Header.Cookies() {
		if string(key) == name {
			previous = append(previous, append([]byte(nil), value...))
		}
	}
	if len(previous) == 0 && sess.Fresh() {
		return sess.SaveWithContext(c)
	}
	// Fiber queues Set-Cookie before storage succeeds, so restore on errors too.
	defer func() {
		c.Response().Header.DelCookie(name)
		for _, value := range previous {
			c.Response().Header.AddBytesV(fiber.HeaderSetCookie, value)
		}
	}()
	return sess.SaveWithContext(c)
}
