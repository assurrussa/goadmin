//nolint:testpackage // Exercises the private auth error boundary.
package adminmiddleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/services/adminservice"
)

func TestExternalRecoveryDenialNeverDispatchesOrRedirects(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			app := fiber.New()
			dispatched := false
			app.Use(func(c fiber.Ctx) error {
				adminservice.EnableExternalAdminRecovery(c)
				return browserAuthFailure(c, adminservice.ErrExternalRecoveryRequired)
			})
			app.All("/command", func(c fiber.Ctx) error {
				dispatched = true
				return c.SendStatus(http.StatusNoContent)
			})
			response, err := app.Test(httptest.NewRequestWithContext(t.Context(), method, "/command", nil))
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, http.StatusUnauthorized, response.StatusCode)
			require.Equal(t, "1", response.Header.Get("X-Goadmin-Auth-Recovery"))
			require.Equal(t, "1", response.Header.Get("X-Goadmin-Auth-Error"))
			require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
			require.Empty(t, response.Header.Get("Location"))
			require.Empty(t, response.Header.Values("Set-Cookie"))
			require.False(t, dispatched)
		})
	}
}

func TestExternalRecoveryMarkerRequiresTrustedHostOptIn(t *testing.T) {
	app := fiber.New()
	app.Post("/command", func(c fiber.Ctx) error {
		return browserAuthFailure(c, adminservice.ErrExternalRecoveryRequired)
	})
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/command", nil)
	request.Header.Set("X-Goadmin-Auth-Recovery", "1")
	response, err := app.Test(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
	require.Empty(t, response.Header.Get("X-Goadmin-Auth-Recovery"))
}
