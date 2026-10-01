//nolint:testpackage // exercises the global auth middleware with real browser cookies.
package adminmiddleware

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/testkit"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/extractors"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/models"
	"github.com/assurrussa/goadmin/services/adminservice"
)

type recoveryAdminRepo struct{}

const (
	recoveryParentDomain = "example.test"
	recoveryLoginPath    = "/auth/login"
)

func (recoveryAdminRepo) GetBySubjectID(context.Context, goauth.SubjectID) (models.Admin, error) {
	panic("ambiguous cookies must never select an authenticated identity")
}

func TestPublicAuthGETRecoversAmbiguousCookies(t *testing.T) {
	for _, domain := range []string{"", recoveryParentDomain} {
		for _, route := range []string{
			recoveryLoginPath, "/auth/register", "/auth/forgot-password", "/reset-password", "/auth/reset-password",
		} {
			t.Run(domain+route, func(t *testing.T) {
				fixture, err := testkit.NewRuntime()
				require.NoError(t, err)
				const name = "admin_session"
				store := session.NewStore(session.Config{
					Extractor: extractors.FromCookie(name), CookieDomain: domain, CookiePath: "/",
					CookieSecure: true, CookieHTTPOnly: true, CookieSameSite: "Lax",
				})
				service := adminservice.NewService(store, nil, adminservice.WithRuntime(fixture.Runtime, recoveryAdminRepo{}))
				app := fiber.New()
				app.Use(AuthAdminMiddleware(service), IsNotAuthAdminMiddleware())
				app.Get(route, func(c fiber.Ctx) error {
					require.Nil(t, GetAdminAuth(c))
					require.Equal(t, "kept", c.Cookies("unrelated"))
					require.Empty(t, c.Cookies(name+"_persistent"))
					sess, getErr := store.Get(c)
					require.NoError(t, getErr)
					defer sess.Release()
					sess.Set("guest", true)
					require.NoError(t, sess.Save())
					return c.SendStatus(http.StatusOK)
				})
				var mutations atomic.Int32
				app.Post("/protected", func(c fiber.Ctx) error {
					mutations.Add(1)
					return c.SendStatus(http.StatusNoContent)
				})
				target, err := url.Parse("https://admin.example.test" + route)
				require.NoError(t, err)
				jar, err := cookiejar.New(nil)
				require.NoError(t, err)
				jar.SetCookies(target, []*http.Cookie{
					{Name: name, Value: "old-host", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode},
					{
						Name: name, Value: "old-domain", Domain: recoveryParentDomain, Path: "/",
						Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
					},
					{Name: name, Value: "old-path", Path: route, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode},
					{
						Name: name + "_persistent", Value: "1", Domain: recoveryParentDomain, Path: "/",
						Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
					},
					{Name: "unrelated", Value: "kept", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode},
				})
				request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target.String(), nil)
				for _, cookie := range jar.Cookies(target) {
					request.AddCookie(cookie)
				}
				response, err := app.Test(request)
				require.NoError(t, err)
				require.NoError(t, response.Body.Close())
				require.Equal(t, http.StatusOK, response.StatusCode)
				require.Equal(t, "no-store", response.Header.Get(fiber.HeaderCacheControl))
				jar.SetCookies(target, response.Cookies())
				var sessions []*http.Cookie
				for _, cookie := range jar.Cookies(target) {
					if cookie.Name == name {
						sessions = append(sessions, cookie)
					}
				}
				require.Len(t, sessions, 1, "all legacy domain/path cookies must disappear")
				require.NotContains(t, sessions[0].Value, "old-")
				request = httptest.NewRequestWithContext(t.Context(), http.MethodPost,
					"https://admin.example.test/protected", strings.NewReader("{}"))
				for _, cookie := range jar.Cookies(target) {
					request.AddCookie(cookie)
				}
				response, err = app.Test(request)
				require.NoError(t, err)
				require.NoError(t, response.Body.Close())
				require.Equal(t, http.StatusUnauthorized, response.StatusCode)
				require.Zero(t, mutations.Load())
			})
		}
	}
}

func TestAmbiguousCookiesStayRejectedOutsidePublicGET(t *testing.T) {
	fixture, err := testkit.NewRuntime()
	require.NoError(t, err)
	service := adminservice.NewService(session.NewStore(), nil, adminservice.WithRuntime(fixture.Runtime, recoveryAdminRepo{}))
	for _, tc := range []struct{ method, route, authorization string }{
		{http.MethodPost, recoveryLoginPath, ""},
		{http.MethodGet, "/protected", ""},
		{http.MethodGet, recoveryLoginPath, "Bearer invalid"},
	} {
		t.Run(tc.method+tc.route+tc.authorization, func(t *testing.T) {
			app := fiber.New()
			app.Use(AuthAdminMiddleware(service), IsNotAuthAdminMiddleware())
			app.All(tc.route, func(fiber.Ctx) error { t.Fatal("invalid credentials reached route"); return nil })
			request := httptest.NewRequestWithContext(t.Context(), tc.method, tc.route, nil)
			request.Header.Set("Cookie", "session_id=first; session_id=second")
			request.Header.Set(fiber.HeaderAuthorization, tc.authorization)
			response, testErr := app.Test(request)
			require.NoError(t, testErr)
			defer response.Body.Close()
			require.Equal(t, http.StatusBadRequest, response.StatusCode)
			require.Empty(t, response.Cookies())
		})
	}
}
