package middlewares

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/google/uuid"

	"github.com/assurrussa/goadmin/internal/httpsecurity"
)

type ConfigRequestID struct {
	Next       func(c fiber.Ctx) bool
	Header     string
	Generator  func() string
	ContextKey any
}

var ConfigRequestIDDefault = ConfigRequestID{
	Header: fiber.HeaderXRequestID, Generator: uuid.NewString, ContextKey: "requestid",
}

func configDefaultRequestID(config ...ConfigRequestID) ConfigRequestID {
	if len(config) == 0 {
		return ConfigRequestIDDefault
	}
	cfg := config[0]
	if cfg.Header == "" {
		cfg.Header = ConfigRequestIDDefault.Header
	}
	if cfg.Generator == nil {
		cfg.Generator = ConfigRequestIDDefault.Generator
	}
	if cfg.ContextKey == nil {
		cfg.ContextKey = ConfigRequestIDDefault.ContextKey
	}
	return cfg
}

// NewRequestID installs Fiber's typed context value used by all audit writers.
// The legacy local is mirrored only for existing logger/host integrations.
func NewRequestID(config ...ConfigRequestID) fiber.Handler {
	cfg := configDefaultRequestID(config...)
	canonical := requestid.New(requestid.Config{Header: cfg.Header, Generator: cfg.Generator})
	return func(c fiber.Ctx) error {
		if cfg.Next != nil && cfg.Next(c) {
			return c.Next()
		}
		rid := c.Get(cfg.Header)
		if !httpsecurity.ValidRequestID(rid) {
			rid = cfg.Generator()
		}
		if !httpsecurity.ValidRequestID(rid) {
			rid = uuid.NewString()
		}
		c.Request().Header.Set(cfg.Header, rid)
		c.Locals(cfg.ContextKey, rid)
		return canonical(c)
	}
}
