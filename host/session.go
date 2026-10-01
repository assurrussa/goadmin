package host

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"

	"github.com/assurrussa/goadmin/infrastructure/core/session/sessionredis"
	redis "github.com/assurrussa/goadmin/infrastructure/redis"
)

// CreateSessionStore exposes the built-in admin session storage wiring behind the stable host package.
func CreateSessionStore(rdb redis.ClientContract, cfg AdminConfig) (fiber.Handler, *session.Store) {
	return sessionredis.CreateAdminSessionStore(rdb, cfg)
}
