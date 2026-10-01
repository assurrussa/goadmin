package http

import (
	"github.com/gofiber/fiber/v3"

	server "github.com/assurrussa/goadmin/infrastructure/fiber/server"
)

// Handler объединяет все API обработчики.
type Handler struct {
	prefix   string
	handlers []server.Handler
}

// NewHandler создает новый главный API обработчик.
func NewHandler(prefix string, handlers ...server.Handler) *Handler {
	return &Handler{
		prefix:   prefix,
		handlers: handlers,
	}
}

// RegisterGroupRoutes регистрирует все API маршруты.
func (h *Handler) RegisterGroupRoutes(route fiber.Router) {
	group := route.Group(h.prefix)
	for _, handler := range h.handlers {
		handler.RegisterGroupRoutes(group)
	}
}
