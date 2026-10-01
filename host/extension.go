package host

import (
	"github.com/gofiber/fiber/v3"

	server "github.com/assurrussa/goadmin/infrastructure/fiber/server"
)

// Extension is a small reusable bootstrap extension implementation for host features.
type Extension struct {
	handlers       []server.Handler
	publicRegister []func(*fiber.App)
	register       []func(*fiber.App)
}

// WithPublicRegister appends host-owned routes that must remain outside the
// embedded admin authentication middleware.
func (e *Extension) WithPublicRegister(registered ...func(*fiber.App)) *Extension {
	e.publicRegister = append(e.publicRegister, registered...)
	return e
}

// NewExtension creates an empty reusable extension.
func NewExtension() *Extension {
	return &Extension{}
}

// WithHandlers appends HTTP handlers that should be mounted into the admin server.
func (e *Extension) WithHandlers(handlers ...server.Handler) *Extension {
	e.handlers = append(e.handlers, handlers...)
	return e
}

// WithRegister appends raw Fiber registration callbacks.
func (e *Extension) WithRegister(registered ...func(*fiber.App)) *Extension {
	e.register = append(e.register, registered...)
	return e
}

// Handlers returns the extension handlers.
func (e *Extension) Handlers() []server.Handler {
	return e.handlers
}

// Register mounts direct Fiber callbacks.
func (e *Extension) Register(app *fiber.App) {
	for _, register := range e.register {
		register(app)
	}
}

// RegisterPublic mounts callbacks before the admin authentication middleware.
func (e *Extension) RegisterPublic(app *fiber.App) {
	for _, register := range e.publicRegister {
		register(app)
	}
}
