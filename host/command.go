package host

import (
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/gofiber/fiber/v3"
)

// Command binds a typed body after current RBAC authorization and supplies only
// the canonical actor. Capture business services in the handler's constructor.
func Command[T any](app *App, key PermissionKey, handle func(fiber.Ctx, Actor, T) error) []fiber.Handler {
	return []fiber.Handler{app.Guard(key.Domain, key.Action), func(c fiber.Ctx) error {
		if integrationroles.RolePreviewActive(c) {
			return c.SendStatus(fiber.StatusForbidden)
		}
		actor, ok := app.CurrentActor(c)
		if !ok {
			return fiber.ErrUnauthorized
		}
		var input T
		if err := c.Bind().Body(&input); err != nil {
			return err
		}
		return handle(c, actor, input)
	}}
}
