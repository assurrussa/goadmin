package bootstrap

import (
	"github.com/gofiber/fiber/v3"

	adminmiddleware "github.com/assurrussa/goadmin/infrastructure/core/middlewares"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
)

// Encrypt protected history before it is stored. Clearing the key on the
// rendered anonymous login page prevents Back from restoring those old props.
// This requires a secure browser context and cannot erase pre-upgrade plaintext.
func adminHistoryMiddleware(manager *goinertia.Inertia) fiber.Handler {
	return func(c fiber.Ctx) error {
		if adminmiddleware.GetAdminAuth(c) != nil {
			manager.WithEncryptHistory(c)
		} else if c.Method() == fiber.MethodGet && c.Path() == "/auth/login" {
			manager.WithClearHistory(c)
		}
		return c.Next()
	}
}
