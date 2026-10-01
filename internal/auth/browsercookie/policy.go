// Package browsercookie enforces the opaque admin browser credential profile.
package browsercookie

import (
	"fmt"
	"strings"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
)

// ErrAmbiguousCookies rejects credentials without choosing one cookie value.
var ErrAmbiguousCookies = fmt.Errorf("ambiguous browser cookies: %w", goauth.ErrInvalidToken)

func Check(c fiber.Ctx, store *session.Store, extraNames ...string) error {
	if c.Get(fiber.HeaderAuthorization) != "" {
		return goauth.ErrInvalidToken
	}
	name := "session_id"
	if store != nil && store.Extractor.Key != "" {
		name = store.Extractor.Key
	}
	names := append([]string{name}, extraNames...)
	for _, n := range names {
		count := 0
		for pair := range strings.SplitSeq(string(c.Request().Header.Peek(fiber.HeaderCookie)), ";") {
			key, _, ok := strings.Cut(strings.TrimSpace(pair), "=")
			if ok && key == n {
				count++
				if count > 1 {
					return ErrAmbiguousCookies
				}
			}
		}
	}
	return nil
}
