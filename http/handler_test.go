package http_test

import (
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"

	"github.com/assurrussa/goadmin/http"
)

func TestAPI_RegisterGroupRoutes(t *testing.T) {
	app := fiber.New()

	h := http.NewHandler("api/v0", &testHandler{})
	assert.NotPanics(t, func() {
		h.RegisterGroupRoutes(app)
	})

	routes := app.GetRoutes()
	assert.NotEmpty(t, routes, "should have routes registered")
	assert.Len(t, routes, 2)
}

type testHandler struct{}

func (h *testHandler) RegisterGroupRoutes(router fiber.Router, _ ...fiber.Handler) {
	router.Get("test", func(_ fiber.Ctx) error { return nil })
	router.Get("test2", func(_ fiber.Ctx) error { return nil })
}
