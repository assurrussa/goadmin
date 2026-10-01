//nolint:testpackage,lll // Tests exercise private security state and keep fixture contracts together.
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
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
	csrfservice "github.com/assurrussa/goadmin/internal/csrf"
	"github.com/assurrussa/goadmin/services/adminservice"
)

func TestAdminCSRFOriginAndOpaqueBinding(t *testing.T) {
	store := newTestSessionStore(t)
	service := newTestCSRFService(t)
	cfg := adminconfig.Config{Domain: testAdminDomain, DomainURL: "https://" + testAdminDomain, CSRFTokenName: testCSRFToken, CSRFTokenTTL: time.Hour, Env: testEnvLocal}
	provider := newCSRFTokenProvider(adminservice.NewService(store, nil), store, service, cfg)
	checker := newCheckCSRFTokenProvider(store, service, cfg)
	app := fiber.New(fiber.Config{ErrorHandler: func(c fiber.Ctx, err error) error {
		var e *goinertia.Error
		if errors.As(err, &e) {
			return c.SendStatus(e.Code)
		}
		return c.SendStatus(500)
	}})
	app.Get("/csrf", func(c fiber.Ctx) error {
		token, err := provider(c)
		if err != nil {
			return err
		}
		return c.SendString(token)
	})
	app.Post("/mutate", func(c fiber.Ctx) error {
		if err := checker(c); err != nil {
			return err
		}
		return c.SendStatus(204)
	})
	resp, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/csrf", nil))
	require.NoError(t, err)
	defer resp.Body.Close()
	var sessionCookie, proof *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == testCSRFToken {
			proof = c
		}
		if c.Name == "session_id" {
			sessionCookie = c
		}
	}
	require.NotNil(t, proof)
	require.NotNil(t, sessionCookie)
	claims, err := service.ExtractClaims(proof.Value)
	require.NoError(t, err)
	require.NotEqual(t, sessionCookie.Value, claims.SessionID)
	require.Equal(t, csrfSessionBinding(sessionCookie.Value), claims.SessionID)
	cases := []struct {
		name, origin, referer, extra string
		code                         int
	}{
		{"exact", "https://" + testAdminDomain, "", "", 204},
		{"referer-only", "", "https://" + testAdminDomain + "/page", "", 204},
		{"missing", "", "", "", 419},
		{"hostile-sibling", "https://evil.localhost", "https://" + testAdminDomain + "/page", "", 419},
		{"null", "null", "https://" + testAdminDomain + "/page", "", 419},
		{"malformed", "https://" + testAdminDomain + "/path", "https://" + testAdminDomain + "/page", "", 419},
		{"duplicate-opaque", "https://" + testAdminDomain, "", "; session_id=other", 419},
		{"duplicate-proof", "https://" + testAdminDomain, "", "; csrf_token=other", 419},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mutate", nil)
			req.Header.Set("Cookie", sessionCookie.Name+"="+sessionCookie.Value+"; "+proof.Name+"="+proof.Value+tc.extra)
			req.Header.Set(csrfservice.HeaderName, proof.Value)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.referer != "" {
				req.Header.Set("Referer", tc.referer)
			}
			res, err := app.Test(req)
			require.NoError(t, err)
			defer res.Body.Close()
			require.Equal(t, tc.code, res.StatusCode)
		})
	}
}
