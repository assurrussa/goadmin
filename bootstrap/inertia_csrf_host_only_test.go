package bootstrap //nolint:testpackage // Tests the private cookie policy boundary.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/require"

	adminconfig "github.com/assurrussa/goadmin/config"
)

const (
	hostScopeIPv6 = "::1"
	hostScopePath = "/admin"
	hostScopeDNS  = "admin.example.test"
)

func TestWriteAdminCSRFCookieKeepsStoreScope(t *testing.T) {
	const proof = "synthetic-csrf-proof"
	const cookieName = "host_scope_csrf"
	const ttl = 37 * time.Minute
	tests := []struct {
		name, origin, tokenDomain, environment string
		store                                  *session.Store
		path, domain                           string
		secure                                 bool
		sameSite                               http.SameSite
		omitted                                bool
	}{
		{
			name: "host-only IPv6", origin: "https://[::1]:19443", tokenDomain: hostScopeIPv6, environment: "production",
			store: &session.Store{Config: session.Config{CookiePath: hostScopePath, CookieSecure: true, CookieSameSite: "Strict"}},
			path:  hostScopePath, secure: true, sameSite: http.SameSiteStrictMode,
		},
		{
			name: "host-only IPv4", origin: "http://127.0.0.1:19443", tokenDomain: "127.0.0.1", environment: "production",
			store: &session.Store{Config: session.Config{CookiePath: "/", CookieSameSite: "Lax"}},
			path:  "/", sameSite: http.SameSiteLaxMode,
		},
		{
			name: "host-only DNS", origin: "https://admin.example.test", tokenDomain: hostScopeDNS, environment: "production",
			store: &session.Store{Config: session.Config{CookiePath: "/account", CookieSecure: true, CookieSameSite: "None"}},
			path:  "/account", secure: true, sameSite: http.SameSiteNoneMode,
		},
		{
			name: "explicit store domain", origin: "https://admin.example.test", tokenDomain: hostScopeDNS,
			environment: "production",
			store: &session.Store{Config: session.Config{
				CookieDomain: "example.test", CookiePath: hostScopePath, CookieSameSite: "Strict",
			}},
			path: hostScopePath, domain: "example.test", sameSite: http.SameSiteStrictMode,
		},
		{
			name: "empty store path and SameSite retain defaults", origin: "http://admin.example.test",
			tokenDomain: hostScopeDNS, environment: "production",
			store: &session.Store{}, path: "/", sameSite: http.SameSiteLaxMode,
		},
		{
			name: "nil store production fallback", origin: "https://admin.example.test",
			tokenDomain: hostScopeDNS, environment: "production",
			path: "/", domain: hostScopeDNS, secure: true, sameSite: http.SameSiteLaxMode,
		},
		{
			name: "nil store local fallback", origin: "http://admin.example.test", tokenDomain: hostScopeDNS, environment: "local",
			path: "/", domain: hostScopeDNS, sameSite: http.SameSiteLaxMode,
		},
		{
			name: "nil store IPv6 behavior unchanged", origin: "https://[::1]:19443",
			tokenDomain: hostScopeIPv6, environment: "production",
			omitted: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := adminconfig.Config{CSRFTokenName: cookieName, CSRFTokenTTL: ttl, Env: test.environment}
			app := fiber.New()
			app.Get(hostScopePath, func(c fiber.Ctx) error {
				writeAdminCSRFCookie(c, test.store, cfg, proof, test.tokenDomain)
				return c.SendStatus(http.StatusOK)
			})
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, test.origin+hostScopePath, nil)
			response, err := app.Test(request)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
			require.Equal(t, http.StatusOK, response.StatusCode)
			if test.omitted {
				require.Empty(t, response.Header.Values("Set-Cookie"))
				return
			}
			require.Len(t, response.Header.Values("Set-Cookie"), 1)
			cookies := response.Cookies()
			require.Len(t, cookies, 1)
			cookie := cookies[0]
			require.Equal(t, cookieName, cookie.Name)
			require.Equal(t, proof, cookie.Value)
			require.Equal(t, test.path, cookie.Path)
			require.Equal(t, test.domain, cookie.Domain)
			require.Equal(t, test.secure, cookie.Secure)
			require.Equal(t, test.sameSite, cookie.SameSite)
			require.False(t, cookie.HttpOnly, "the client must be able to copy its CSRF proof into the request header")
			require.Equal(t, int(ttl.Seconds()), cookie.MaxAge)
			require.NoError(t, cookie.Valid())
			if test.domain == "" {
				require.NotContains(t, strings.ToLower(response.Header.Get("Set-Cookie")), "; domain=",
					"host-only session policy must not widen CSRF cookie scope")
			}
		})
	}
}

func TestAdminCSRFCookieIPv6DomainRejectedBeforeSerialization(t *testing.T) {
	cookie := &http.Cookie{
		Name: "synthetic_csrf", Value: "synthetic-proof", Domain: hostScopeIPv6, Path: "/",
		Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode,
	}
	require.Error(t, cookie.Valid(), "the pinned Go validator rejects colon-containing cookie domains")
	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error {
		c.Cookie(&fiber.Cookie{Name: cookie.Name, Value: cookie.Value, Domain: cookie.Domain, Path: cookie.Path, Secure: true})
		return c.SendStatus(http.StatusOK)
	})
	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://[::1]:19443/", nil))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
	require.Empty(t, response.Header.Values("Set-Cookie"), "Fiber silently omits the entire invalid cookie")
}
