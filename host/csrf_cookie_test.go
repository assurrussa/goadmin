package host_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	adminhost "github.com/assurrussa/goadmin/host"
)

func TestHostCSRFCookieUsesNativeSessionScope(t *testing.T) {
	fixture := adminappt.NewAppTest(t)
	fixture.SessionStore.CookieDomain = "example.test"
	fixture.SessionStore.CookiePath = "/"
	fixture.SessionStore.CookieSecure = true
	fixture.SessionStore.CookieSameSite = "Lax"
	app := adminhost.WrapApp(fixture.App)
	server := fiber.New()
	server.Get("/csrf", func(c fiber.Ctx) error {
		require.NoError(t, app.WriteAdminCSRFCookie(c, "csrf_token", "synthetic-proof", time.Hour))
		return c.SendStatus(http.StatusOK)
	})
	response, err := server.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://admin.example.test/csrf", nil))
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Len(t, response.Cookies(), 1)
	cookie := response.Cookies()[0]
	require.Equal(t, "example.test", cookie.Domain)
	require.Equal(t, "/", cookie.Path)
	require.True(t, cookie.Secure)
	require.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
	require.Equal(t, 3600, cookie.MaxAge)
}
