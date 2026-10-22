//nolint:testpackage // Exercises the actual Inertia middleware and CSRF provider together.
package bootstrap

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	adminconfig "github.com/assurrussa/goadmin/config"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
	csrfservice "github.com/assurrussa/goadmin/internal/csrf"
	"github.com/assurrussa/goadmin/services/adminservice"
	"github.com/assurrussa/goadmin/views"
)

func TestStaleFormProofSurvivesInertiaCSRFRendering(t *testing.T) {
	store := newTestSessionStore(t)
	csrf := newTestCSRFService(t)
	cfg := adminconfig.Config{
		Domain: testAdminDomain, DomainURL: "https://" + testAdminDomain,
		CSRFTokenName: testCSRFToken, CSRFTokenTTL: time.Hour, Env: testEnvLocal,
	}
	provider := newCSRFTokenProvider(adminservice.NewService(store, nil), store, csrf, cfg)
	inertia, err := goinertia.NewWithValidation(cfg.DomainURL,
		goinertia.WithFS(views.Templates),
		goinertia.WithCSRFTokenProvider(provider),
		goinertia.WithCSRFTokenCheckProvider(newCheckCSRFTokenProvider(store, csrf, cfg)))
	require.NoError(t, err)
	app := fiber.New(fiber.Config{ErrorHandler: func(c fiber.Ctx, err error) error {
		var typed *goinertia.Error
		if errors.As(err, &typed) {
			return c.SendStatus(typed.Code)
		}
		return fiber.DefaultErrorHandler(c, err)
	}})
	app.Get("/csrf", func(c fiber.Ctx) error { _, err := provider(c); return err })
	app.Use(inertia.Middleware())
	app.Get("/sso-link", func(c fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}
		id := sess.ID()
		sess.Release()
		token, err := csrf.Create(c, csrfservice.Request{SessionID: CSRFSessionBinding(id), Domain: cfg.Domain})
		if err != nil {
			return err
		}
		c.Cookie(&fiber.Cookie{
			Name: testCSRFToken, Value: token.Token, Path: "/", Secure: true,
			SameSite: fiber.CookieSameSiteLaxMode,
		})
		return inertia.Render(c, "auth/SSOErrorPage", map[string]any{
			"reauthCSRF": token.Token, testCSRFToken: token.Token,
		})
	})
	calls := 0
	app.Post("/sso-link/reauth", func(c fiber.Ctx) error {
		calls++
		return c.SendStatus(http.StatusNoContent)
	})
	initial, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/csrf", nil))
	require.NoError(t, err)
	defer initial.Body.Close()
	var sessionCookie *http.Cookie
	for _, cookie := range initial.Cookies() {
		if cookie.Name == realtimeSessionCookieName {
			sessionCookie = cookie
		}
	}
	require.NotNil(t, sessionCookie)

	for _, staleCookie := range []string{"", "invalid"} {
		t.Run("incoming-proof-"+staleCookie, func(t *testing.T) {
			get := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/sso-link", nil)
			get.AddCookie(sessionCookie)
			if staleCookie != "" {
				get.AddCookie(&http.Cookie{ //nolint:gosec // CSRF is deliberately browser-readable.
					Name: testCSRFToken, Value: staleCookie, Secure: true,
				})
			}
			get.Header.Set("X-Inertia", "true")
			response, err := app.Test(get)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, http.StatusOK, response.StatusCode)
			var page struct {
				Props map[string]any `json:"props"`
			}
			require.NoError(t, json.NewDecoder(response.Body).Decode(&page))
			formProof, ok := page.Props["reauthCSRF"].(string)
			require.True(t, ok)
			require.NotEmpty(t, formProof)
			require.Equal(t, formProof, page.Props[testCSRFToken])
			var emitted *http.Cookie
			for _, cookie := range response.Cookies() {
				if cookie.Name == testCSRFToken {
					emitted = cookie
				}
			}
			require.NotNil(t, emitted)
			require.Equal(t, formProof, emitted.Value)
			for _, invalid := range []struct {
				name, origin, header string
			}{
				{"missing-header", cfg.DomainURL, ""},
				{"wrong-header", cfg.DomainURL, "invalid"},
				{"cross-origin", "https://evil.invalid", formProof},
			} {
				t.Run(invalid.name, func(t *testing.T) {
					denied := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
						cfg.DomainURL+"/sso-link/reauth", nil)
					denied.AddCookie(sessionCookie)
					denied.AddCookie(emitted)
					denied.Header.Set("Origin", invalid.origin)
					if invalid.header != "" {
						denied.Header.Set(csrfservice.HeaderName, invalid.header)
					}
					result, err := app.Test(denied)
					require.NoError(t, err)
					defer result.Body.Close()
					require.NotEqual(t, http.StatusNoContent, result.StatusCode)
				})
			}
			post := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
				cfg.DomainURL+"/sso-link/reauth", nil)
			post.AddCookie(sessionCookie)
			post.AddCookie(emitted)
			post.Header.Set("Origin", cfg.DomainURL)
			post.Header.Set(csrfservice.HeaderName, formProof)
			result, err := app.Test(post)
			require.NoError(t, err)
			defer result.Body.Close()
			require.Equal(t, http.StatusNoContent, result.StatusCode)
		})
	}
	require.Equal(t, 2, calls)
}
