//nolint:testpackage // verifies the production auth and CSRF middleware together.
package bootstrap

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/testkit"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/require"

	adminconfig "github.com/assurrussa/goadmin/config"
	adminmiddleware "github.com/assurrussa/goadmin/infrastructure/core/middlewares"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
	csrfservice "github.com/assurrussa/goadmin/internal/csrf"
	"github.com/assurrussa/goadmin/models"
	"github.com/assurrussa/goadmin/services/adminservice"
)

type anonymousRecoveryRepo struct{}

func (anonymousRecoveryRepo) GetBySubjectID(context.Context, goauth.SubjectID) (models.Admin, error) {
	panic("cookie recovery must remain anonymous")
}

func TestPublicAuthCookieRecoveryIssuesUsableCSRFProof(t *testing.T) {
	for _, duplicate := range []string{"session_id", testCSRFToken} {
		t.Run(duplicate, func(t *testing.T) {
			fixture, err := testkit.NewRuntime()
			require.NoError(t, err)
			store := session.NewStore(session.Config{CookiePath: "/", CookieSecure: true, CookieHTTPOnly: true})
			service := adminservice.NewService(store, nil, adminservice.WithRuntime(fixture.Runtime, anonymousRecoveryRepo{}))
			cfg := adminconfig.Config{
				Domain: testAdminDomain, DomainURL: "https://" + testAdminDomain,
				CSRFTokenName: testCSRFToken, CSRFTokenTTL: time.Hour,
			}
			provider := newCSRFTokenProvider(service, store, newTestCSRFService(t), cfg)
			checker := newCheckCSRFTokenProvider(store, newTestCSRFService(t), cfg)
			app := fiber.New(fiber.Config{ErrorHandler: func(c fiber.Ctx, failure error) error {
				var problem *goinertia.Error
				if errors.As(failure, &problem) {
					return c.SendStatus(problem.Code)
				}
				return fiber.DefaultErrorHandler(c, failure)
			}})
			app.Use(adminmiddleware.AuthAdminMiddleware(service, testCSRFToken), adminmiddleware.IsNotAuthAdminMiddleware())
			app.Get("/auth/login", func(c fiber.Ctx) error {
				token, tokenErr := provider(c)
				if tokenErr != nil {
					return tokenErr
				}
				return c.SendString(token)
			})
			app.Post("/auth/login", func(c fiber.Ctx) error {
				if checkErr := checker(c); checkErr != nil {
					return checkErr
				}
				return c.SendStatus(http.StatusNoContent)
			})
			target, err := url.Parse(cfg.DomainURL + "/auth/login")
			require.NoError(t, err)
			jar, err := cookiejar.New(nil)
			require.NoError(t, err)
			jar.SetCookies(target, []*http.Cookie{
				{Name: duplicate, Value: "old-root", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode},
				{Name: duplicate, Value: "old-path", Path: "/auth", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode},
			})
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target.String(), nil)
			for _, cookie := range jar.Cookies(target) {
				request.AddCookie(cookie)
			}
			response, err := app.Test(request)
			require.NoError(t, err)
			proof, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, http.StatusOK, response.StatusCode)
			jar.SetCookies(target, response.Cookies())
			request = httptest.NewRequestWithContext(t.Context(), http.MethodPost, target.String(), nil)
			for _, cookie := range jar.Cookies(target) {
				request.AddCookie(cookie)
			}
			request.Header.Set(fiber.HeaderOrigin, cfg.DomainURL)
			request.Header.Set(csrfservice.HeaderName, string(proof))
			response, err = app.Test(request)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, http.StatusNoContent, response.StatusCode, "fresh proof must allow login without clearing cookies manually")
		})
	}
}
