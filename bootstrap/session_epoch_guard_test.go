package bootstrap //nolint:testpackage // Exercises canonical cookies, middleware and real CSRF before a command.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	fibersession "github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/require"

	adminconfig "github.com/assurrussa/goadmin/config"
	adminmiddleware "github.com/assurrussa/goadmin/infrastructure/core/middlewares"
)

func TestDocumentEpochRefusesAnotherLoginBeforeBusinessHandler(t *testing.T) {
	const commandPath = "/command"
	store := fibersession.NewStore()
	f := newRealtimeLogoutFixture(t, store)
	cfg := adminconfig.Config{Domain: testAdminDomain, DomainURL: "https://" + testAdminDomain, CSRFTokenName: testCSRFToken}
	csrf := newTestCSRFService(t)
	deps := Dependencies{System: SystemDependencies{AdminAuthService: f.service, CSRF: csrf, SessionStore: store}}
	f.app.Get("/auth/session/maintain", sessionMaintenanceHandler(deps, cfg))
	login := func() []*http.Cookie {
		response, err := f.app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/login", nil))
		require.NoError(t, err)
		defer response.Body.Close()
		require.Equal(t, http.StatusNoContent, response.StatusCode)
		return response.Cookies()
	}
	settle := func(cookies []*http.Cookie) ([]*http.Cookie, string) {
		for step := 0; step < 2; step++ {
			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, cfg.DomainURL+"/auth/session/maintain", nil)
			r.Header.Set(maintenanceHeader, "1")
			r.Header.Set("Origin", cfg.DomainURL)
			for _, cookie := range cookies {
				r.AddCookie(cookie)
			}
			response, err := f.app.Test(r)
			require.NoError(t, err)
			var body sessionMaintenanceResponse
			require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
			require.NoError(t, response.Body.Close())
			cookies = append(cookies, response.Cookies()...)
			if response.StatusCode == http.StatusOK {
				require.NotNil(t, body.Maintenance)
				return cookies, body.Maintenance.SessionEpoch
			}
			require.Equal(t, http.StatusConflict, response.StatusCode)
		}
		t.Fatal("maintenance did not settle")
		return nil, ""
	}
	_, epochA := settle(login())
	cookiesB, epochB := settle(login())
	require.NotEqual(t, epochA, epochB, "independent login of the same admin changes the epoch")
	var csrfB string
	for _, cookie := range cookiesB {
		if cookie.Name == testCSRFToken {
			csrfB = cookie.Value
		}
	}
	require.NotEmpty(t, csrfB)
	called := 0
	app := fiber.New()
	app.Use(adminmiddleware.AuthAdminMiddleware(f.service, testCSRFToken), adminmiddleware.IsNotAuthAdminMiddleware())
	checker := newCheckCSRFTokenProvider(store, csrf, cfg)
	app.All(commandPath, func(c fiber.Ctx) error {
		if c.Method() == http.MethodPost {
			if err := checker(c); err != nil {
				return err
			}
		}
		called++
		return c.SendStatus(http.StatusNoContent)
	})
	app.Post("/auth/logout", func(c fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })
	for _, tc := range []struct {
		name, method, path string
		versions, epochs   []string
		status             int
	}{
		{"old document new cookies and csrf", http.MethodPost, commandPath, []string{"1"}, []string{epochA}, http.StatusConflict},
		{"old document read", http.MethodGet, commandPath, []string{"1"}, []string{epochA}, http.StatusConflict},
		{"missing", http.MethodPost, commandPath, []string{"1"}, nil, http.StatusConflict},
		{"malformed", http.MethodPost, commandPath, []string{"1"}, []string{"bad"}, http.StatusConflict},
		{"duplicate", http.MethodPost, commandPath, []string{"1"}, []string{epochB, epochB}, http.StatusConflict},
		{"duplicate negotiation", http.MethodPost, commandPath, []string{"1", "1"}, []string{epochB}, http.StatusConflict},
		{"current document", http.MethodPost, commandPath, []string{"1"}, []string{epochB}, http.StatusNoContent},
		{"legacy client", http.MethodPost, commandPath, nil, nil, http.StatusNoContent},
		{"explicit logout", http.MethodPost, "/auth/logout", []string{"1"}, nil, http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := called
			r := httptest.NewRequestWithContext(t.Context(), tc.method, cfg.DomainURL+tc.path, nil)
			r.Header.Set("Origin", cfg.DomainURL)
			r.Header.Set("X-CSRF-Token", csrfB)
			r.Header.Set("X-Inertia", "true")
			for _, v := range tc.versions {
				r.Header.Add("X-Goadmin-Session-Maintenance", v)
			}
			for _, v := range tc.epochs {
				r.Header.Add("X-Goadmin-Session-Epoch", v)
			}
			for _, cookie := range cookiesB {
				r.AddCookie(cookie)
			}
			response, err := app.Test(r)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, tc.status, response.StatusCode)
			if tc.status == http.StatusConflict {
				require.Equal(t, before, called, "no business effect before refusal")
				require.Equal(t, "1", response.Header.Get("X-Goadmin-Auth-Error"))
				require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
				require.Empty(t, response.Header.Get("Location"))
				require.Empty(t, response.Header.Get("X-Inertia"))
				require.Empty(t, response.Cookies())
				var body sessionMaintenanceResponse
				require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
				require.Equal(t, "session_changed", body.Code)
			} else if tc.path == commandPath {
				require.Equal(t, before+1, called)
			}
		})
	}
}
