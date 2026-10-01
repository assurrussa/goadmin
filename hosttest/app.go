package hosttest

import (
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	"github.com/assurrussa/goadmin/infrastructure/core/session"
	"github.com/assurrussa/goadmin/models"
)

type App = adminappt.App

// NewApp creates the standard admin host test harness.
func NewApp(t *testing.T) *App {
	t.Helper()

	return adminappt.NewAppTest(t)
}

// WithSessionAdmin injects a minimal authenticated admin session into request locals.
func WithSessionAdmin(adminID int64) fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Locals(session.AuthAdminKey.String(), &models.SessionAdmin{ID: adminID})
		return c.Next()
	}
}
