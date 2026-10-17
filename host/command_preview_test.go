package host //nolint:testpackage // Exercises request-local preview command fence.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
)

func TestRolePreviewDeniesCommandOnGETBeforeBindingOrExecution(t *testing.T) {
	fixture := adminappt.NewAppTest(t)
	key := NewPermissionKey("users", PermissionActionRead)
	fixture.MockPermissionGuard.EXPECT().AdminGuard(key).Return(func(c fiber.Ctx) error { return c.Next() })
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.SetContext(integrationroles.WithSuperRolePreview(c.Context()))
		return c.Next()
	})
	calls := 0
	handlers := Command(WrapApp(fixture.App), key, func(c fiber.Ctx, _ Actor, _ struct{ Value string }) error {
		calls++
		return c.SendStatus(204)
	})
	app.Get("/command", handlers[0], handlers[1])
	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/command", nil))
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusForbidden, response.StatusCode)
	require.Zero(t, calls)
}
