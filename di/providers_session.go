package di

import (
	"github.com/gofiber/fiber/v3/middleware/session"

	"github.com/assurrussa/goadmin/bootstrap"
	"github.com/assurrussa/goadmin/infrastructure/core/session/sessionredis"
	redis "github.com/assurrussa/goadmin/infrastructure/redis"
)

func provideAdminSessionStore(pool redis.ClientContract, cfg bootstrap.Config) (*session.Store, error) {
	_, store := sessionredis.CreateAdminSessionStore(pool, cfg.AdminConfig)

	return store, nil
}
