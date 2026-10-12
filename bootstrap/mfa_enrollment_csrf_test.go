//nolint:testpackage // Verifies enrollment routes against the real anonymous-auth and CSRF middleware.
package bootstrap

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	adminconfig "github.com/assurrussa/goadmin/config"
	adminmiddleware "github.com/assurrussa/goadmin/infrastructure/core/middlewares"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
	csrfservice "github.com/assurrussa/goadmin/internal/csrf"
	"github.com/assurrussa/goadmin/services/adminservice"
	"github.com/assurrussa/goadmin/views"
)

func TestRequiredEnrollmentPOSTsRetainCSRFAndOriginChecks(t *testing.T) {
	store := newTestSessionStore(t)
	csrf := newTestCSRFService(t)
	cfg := adminconfig.Config{
		Domain: testAdminDomain, DomainURL: "https://" + testAdminDomain,
		CSRFTokenName: testCSRFToken, CSRFTokenTTL: time.Hour, Env: testEnvLocal,
	}
	provider := newCSRFTokenProvider(adminservice.NewService(store, nil), store, csrf, cfg)
	inertia, err := goinertia.NewWithValidation(cfg.DomainURL,
		goinertia.WithFS(views.Templates),
		goinertia.WithCSRFTokenCheckProvider(newCheckCSRFTokenProvider(store, csrf, cfg)))
	require.NoError(t, err)
	app := fiber.New(fiber.Config{ErrorHandler: func(c fiber.Ctx, err error) error {
		var typed *goinertia.Error
		if errors.As(err, &typed) {
			return c.SendStatus(typed.Code)
		}
		return fiber.DefaultErrorHandler(c, err)
	}})
	app.Get("/csrf", func(c fiber.Ctx) error {
		_, err := provider(c)
		if err != nil {
			return err
		}
		return c.SendStatus(http.StatusNoContent)
	})
	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/csrf", nil))
	require.NoError(t, err)
	cookies := response.Cookies()
	require.NoError(t, response.Body.Close())
	var sessionCookie, proof *http.Cookie
	for _, cookie := range cookies {
		switch cookie.Name {
		case realtimeSessionCookieName:
			sessionCookie = cookie
		case testCSRFToken:
			proof = cookie
		}
	}
	require.NotNil(t, sessionCookie)
	require.NotNil(t, proof)

	// Same order and actual middleware used for the built-in anonymous auth routes.
	app.Use(adminmiddleware.IsNotAuthAdminMiddleware())
	app.Use(inertia.Middleware())
	calls := 0
	paths := []string{"/auth/mfa/enrollment/begin", "/auth/mfa/enrollment/confirm"}
	for _, path := range paths {
		app.Post(path, func(c fiber.Ctx) error {
			calls++
			return c.SendStatus(http.StatusNoContent)
		})
	}
	for _, path := range paths {
		for _, tc := range []struct {
			name      string
			origin    string
			header    bool
			duplicate bool
			status    int
		}{
			{"valid", cfg.DomainURL, true, false, http.StatusNoContent},
			{"missing-proof", cfg.DomainURL, false, false, 419},
			{"missing-origin", "", true, false, 419},
			{"hostile-origin", "https://evil.example.test", true, false, 419},
			{"duplicate-proof", cfg.DomainURL, true, true, 419},
		} {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				before := calls
				request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, nil)
				request.AddCookie(sessionCookie)
				request.AddCookie(proof)
				if tc.duplicate {
					request.Header.Set("Cookie", request.Header.Get("Cookie")+"; "+testCSRFToken+"=different")
				}
				if tc.header {
					request.Header.Set(csrfservice.HeaderName, proof.Value)
				}
				if tc.origin != "" {
					request.Header.Set("Origin", tc.origin)
				}
				response, err := app.Test(request)
				require.NoError(t, err)
				defer response.Body.Close()
				require.Equal(t, tc.status, response.StatusCode)
				if tc.status == http.StatusNoContent {
					require.Equal(t, before+1, calls)
				} else {
					require.Equal(t, before, calls)
				}
			})
		}
		before := calls
		response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		require.Equal(t, http.StatusMethodNotAllowed, response.StatusCode)
		require.Equal(t, before, calls)
	}
}
