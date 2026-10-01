package middlewares_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/infrastructure/fiber/middlewares"
)

func TestRequestIDPropagatesOrGenerates(t *testing.T) {
	for _, supplied := range []string{"", "caller-id"} {
		t.Run(supplied, func(t *testing.T) {
			app := fiber.New()
			app.Use(middlewares.NewRequestID(middlewares.ConfigRequestID{Generator: func() string { return "generated-id" }}))
			app.Get("/", func(c fiber.Ctx) error {
				require.Equal(t, c.Get(fiber.HeaderXRequestID), c.Locals("requestid"))
				return c.SendStatus(http.StatusNoContent)
			})
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			request.Header.Set(fiber.HeaderXRequestID, supplied)
			response, err := app.Test(request)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			want := supplied
			if want == "" {
				want = "generated-id"
			}
			require.Equal(t, want, response.Header.Get(fiber.HeaderXRequestID))
		})
	}
}
