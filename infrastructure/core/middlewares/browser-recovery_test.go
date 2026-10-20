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
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
			request.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.8")
			request.Header.Set("Sec-Fetch-Mode", "navigate")
			request.Header.Set("Sec-Fetch-Dest", "document")
			response, err := app.Test(request)
			require.NoError(t, err)
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.Equal(t, http.StatusUnauthorized, response.StatusCode)
			require.Contains(t, response.Header.Get("Content-Type"), fiber.MIMETextHTML)
			require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
			require.Equal(t, "1", response.Header.Get("X-Goadmin-Auth-Recovery"))
			require.Empty(t, response.Header.Values("Set-Cookie"))
			require.Empty(t, response.Header.Get("Location"))
			require.Contains(t, string(body), `src="/public/auth-recovery.js"`)
			require.Contains(t, string(body), `href="/public/auth-recovery.css"`)
			require.Contains(t, string(body), `aria-live="polite"`)
			require.NotContains(t, string(body), "<style>")
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
		{"head", http.MethodHead, fiber.MIMETextHTML, "", ""},
		{"post", http.MethodPost, fiber.MIMETextHTML, "", ""},
		{"patch", http.MethodPatch, fiber.MIMETextHTML, "", ""},
		{"delete", http.MethodDelete, fiber.MIMETextHTML, "", ""},
		{"api", http.MethodGet, "application/json", "", ""},
		{"mixed api", http.MethodGet, "text/html,application/json", "", ""},
		{"wildcard", http.MethodGet, "*/*", "", ""},
		{"html forbidden", http.MethodGet, "text/html;q=0", "", ""},
		{"html zero quality", http.MethodGet, "text/html;q=0.000", "", ""},
		{"html invalid quality", http.MethodGet, "text/html;q=NaN", "", ""},
		{"inertia", http.MethodGet, fiber.MIMETextHTML, "X-Inertia", "true"},
		{"xhr", http.MethodGet, fiber.MIMETextHTML, "X-Requested-With", "XMLHttpRequest"},
		{"fetch", http.MethodGet, fiber.MIMETextHTML, "Sec-Fetch-Mode", "cors"},
		{"resource", http.MethodGet, fiber.MIMETextHTML, "Sec-Fetch-Dest", "script"},
		{"authorization", http.MethodGet, fiber.MIMETextHTML, "Authorization", "Bearer synthetic"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()
			app.Use(func(c fiber.Ctx) error {
				adminservice.EnableExternalAdminRecovery(c)
				return browserAuthFailure(c, adminservice.ErrExternalRecoveryRequired)
			})
			r := httptest.NewRequestWithContext(t.Context(), tc.method, "/command", strings.NewReader("must-not-replay"))
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
	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
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
	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/public/auth-recovery.js", nil))
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Contains(t, response.Header.Get("Content-Type"), "javascript")
}

func TestRecoveryStylesDoNotRequireBrowserAuthority(t *testing.T) {
	app := fiber.New()
	app.Get("/public/auth-recovery.css", BrowserAuthRecoveryStyles)
	app.Use(func(c fiber.Ctx) error { return c.SendStatus(http.StatusForbidden) })
	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/public/auth-recovery.css", nil))
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Contains(t, response.Header.Get("Content-Type"), "text/css")
	require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "prefers-color-scheme: dark")
	require.NotContains(t, string(body), "@import")
	require.NotContains(t, string(body), "url(")
}
