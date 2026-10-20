//nolint:testpackage // Exercises the private browser transport boundary.
package adminmiddleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/services/adminservice"
)

func TestExternalRecoveryDocumentNavigation(t *testing.T) {
	for _, target := range []string{"/admin/sites/authhub/view", "/auth/login?redirectback=%2Fadmin%2Fsites"} {
		t.Run(target, func(t *testing.T) {
			app := fiber.New()
			app.Use(func(c fiber.Ctx) error {
				adminservice.EnableExternalAdminRecovery(c)
				adminservice.SetExternalAdminRecoverySignIn(c, "/sso?login=1")
				return browserAuthFailure(c, adminservice.ErrExternalRecoveryRequired)
			})
			request := httptest.NewRequest(http.MethodGet, target, nil)
			request.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.8")
			request.Header.Set("Sec-Fetch-Mode", "navigate")
			request.Header.Set("Sec-Fetch-Dest", "document")
			response, err := app.Test(request)
			require.NoError(t, err)
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.Equal(t, http.StatusUnauthorized, response.StatusCode)
			require.Contains(t, response.Header.Get("Content-Type"), "text/html")
			require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
			require.Equal(t, "1", response.Header.Get("X-Goadmin-Auth-Recovery"))
			require.Empty(t, response.Header.Values("Set-Cookie"))
			require.Empty(t, response.Header.Get("Location"))
			require.Contains(t, string(body), `src="/public/auth-recovery.js"`)
			require.Contains(t, string(body), `href="/sso?login=1"`)
			require.NotContains(t, string(body), `href="/auth/login`)
			require.NotContains(t, string(body), "<script>")
		})
	}
}

func TestExternalRecoveryNonDocumentContractUnchanged(t *testing.T) {
	cases := []struct {
		name, method, accept, header, value string
	}{
		{"head", "HEAD", "text/html", "", ""},
		{"post", "POST", "text/html", "", ""},
		{"patch", "PATCH", "text/html", "", ""},
		{"delete", "DELETE", "text/html", "", ""},
		{"api", "GET", "application/json", "", ""},
		{"mixed api", "GET", "text/html,application/json", "", ""},
		{"wildcard", "GET", "*/*", "", ""},
		{"html forbidden", "GET", "text/html;q=0", "", ""},
		{"html zero quality", "GET", "text/html;q=0.000", "", ""},
		{"html invalid quality", "GET", "text/html;q=NaN", "", ""},
		{"inertia", "GET", "text/html", "X-Inertia", "true"},
		{"xhr", "GET", "text/html", "X-Requested-With", "XMLHttpRequest"},
		{"fetch", "GET", "text/html", "Sec-Fetch-Mode", "cors"},
		{"resource", "GET", "text/html", "Sec-Fetch-Dest", "script"},
		{"authorization", "GET", "text/html", "Authorization", "Bearer synthetic"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()
			app.Use(func(c fiber.Ctx) error {
				adminservice.EnableExternalAdminRecovery(c)
				return browserAuthFailure(c, adminservice.ErrExternalRecoveryRequired)
			})
			r := httptest.NewRequest(tc.method, "/command", strings.NewReader("must-not-replay"))
			r.Header.Set("Accept", tc.accept)
			if tc.header != "" {
				r.Header.Set(tc.header, tc.value)
			}
			response, err := app.Test(r)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, http.StatusUnauthorized, response.StatusCode)
			require.Contains(t, response.Header.Get("Content-Type"), "application/json")
			require.Equal(t, "1", response.Header.Get("X-Goadmin-Auth-Error"))
			require.Empty(t, response.Header.Get("Location"))
			require.Empty(t, response.Header.Values("Set-Cookie"))
		})
	}
}

func TestRecoverySignInRejectsUnsafeAndLoopTargets(t *testing.T) {
	for _, target := range []string{
		"", "https://evil.test/login", "//evil.test", "/\\evil.test", "/%2f%2fevil.test", "/./sso", "/sso/../auth/login",
		"/auth/login", "/auth/login?redirectback=/", "/auth/session/recover", "/sso#fragment", "/sso\r\nLocation: evil",
		"/auth/login/", "/AUTH/LOGIN", "/auth//login", "/auth/session/recover/", "/AUTH/SESSION/RECOVER", "/auth//session/recover",
	} {
		require.Empty(t, safeRecoverySignIn(target), target)
	}
	require.Equal(t, "/sso?login=1", safeRecoverySignIn("/sso?login=1"))
}

func TestRecoveryWithoutHostSignInDoesNotInventLoginLink(t *testing.T) {
	app := fiber.New()
	app.Get("/", sendBrowserRecoveryDocument)
	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NotContains(t, string(body), "auth-recovery-sign-in")
	require.NotContains(t, string(body), "/auth/login")
}

func TestRecoveryScriptDoesNotRequireBrowserAuthority(t *testing.T) {
	app := fiber.New()
	app.Get("/public/auth-recovery.js", BrowserAuthRecoveryScript)
	app.Use(func(c fiber.Ctx) error { return c.SendStatus(http.StatusForbidden) })
	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/public/auth-recovery.js", nil))
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Contains(t, response.Header.Get("Content-Type"), "javascript")
}
