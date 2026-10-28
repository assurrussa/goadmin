package bootstrap //nolint:testpackage // Exercises the native auth/CSRF boundary.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	fibersession "github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/require"

	adminconfig "github.com/assurrussa/goadmin/config"
	adminmiddleware "github.com/assurrussa/goadmin/infrastructure/core/middlewares"
	coresession "github.com/assurrussa/goadmin/infrastructure/core/session"
)

func TestSessionMaintenanceConfirmsCookiesAndKeepsLoginLimits(t *testing.T) {
	store := fibersession.NewStore()
	f := newRealtimeLogoutFixture(t, store)
	cfg := adminconfig.Config{
		Domain: testAdminDomain, DomainURL: "https://" + testAdminDomain,
		CSRFTokenName: testCSRFToken, CSRFTokenTTL: time.Hour,
	}
	deps := Dependencies{System: SystemDependencies{AdminAuthService: f.service, CSRF: newTestCSRFService(t), SessionStore: store}}
	f.app.All("/auth/session/maintain", sessionMaintenanceHandler(deps, cfg))
	login, err := f.app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/login", nil))
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, login.StatusCode)
	defer login.Body.Close()
	cookies := login.Cookies()
	require.NotEmpty(t, cookies)
	request := func(method, origin string) *http.Response {
		r := httptest.NewRequestWithContext(t.Context(), method, cfg.DomainURL+"/auth/session/maintain", nil)
		r.Header.Set(maintenanceHeader, "1")
		r.Header.Set("Origin", origin)
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		response, testErr := f.app.Test(r)
		require.NoError(t, testErr)
		return response
	}
	for _, tc := range []struct{ method, origin string }{
		{http.MethodPost, cfg.DomainURL}, {http.MethodGet, "https://foreign.test"},
	} {
		response := request(tc.method, tc.origin)
		require.Equal(t, http.StatusForbidden, response.StatusCode)
		require.Empty(t, response.Cookies())
		_ = response.Body.Close()
	}
	first := request(http.MethodGet, cfg.DomainURL)
	require.Equal(t, http.StatusConflict, first.StatusCode)
	require.Equal(t, "no-store", first.Header.Get("Cache-Control"))
	issued := first.Cookies()
	require.Len(t, issued, 1)
	require.Equal(t, testCSRFToken, issued[0].Name)
	cookies = append(cookies, issued...)
	_ = first.Body.Close()
	var before sessionMaintenanceResponse
	second := request(http.MethodGet, cfg.DomainURL)
	require.Equal(t, http.StatusOK, second.StatusCode)
	require.Empty(t, second.Cookies())
	require.NoError(t, json.NewDecoder(second.Body).Decode(&before))
	_ = second.Body.Close()
	require.True(t, before.CSRFReady)
	require.NotNil(t, before.Maintenance)
	require.NotEmpty(t, before.Maintenance.SessionEpoch)
	require.Positive(t, before.Maintenance.NextCheckAfterMS)
	third := request(http.MethodGet, cfg.DomainURL)
	var after sessionMaintenanceResponse
	require.Equal(t, http.StatusOK, third.StatusCode)
	require.Empty(t, third.Cookies(), "healthy maintenance must not reissue credentials")
	require.NoError(t, json.NewDecoder(third.Body).Decode(&after))
	_ = third.Body.Close()
	require.Equal(t, before.Maintenance.HardExpiresAt, after.Maintenance.HardExpiresAt)
	require.Equal(t, before.Maintenance.SessionEpoch, after.Maintenance.SessionEpoch)
	require.Equal(t, before.Maintenance.Revision, after.Maintenance.Revision)
}

func TestCSRFRepairRefusesMutationBeforeBusinessHandler(t *testing.T) {
	store := fibersession.NewStore()
	f := newRealtimeLogoutFixture(t, store)
	cfg := adminconfig.Config{Domain: testAdminDomain, DomainURL: "https://" + testAdminDomain, CSRFTokenName: testCSRFToken}
	checker := newCheckCSRFTokenProvider(store, newTestCSRFService(t), cfg)
	called := 0
	app := fiber.New(fiber.Config{ErrorHandler: sessionMaintenanceErrorHandler(fiber.DefaultErrorHandler)})
	app.Post("/command", func(c fiber.Ctx) error {
		c.Locals(coresession.AuthAdminKey.String(), &f.admin)
		// Only canonical authenticated SessionAdmin locals authorize repair.
		admin, err := f.service.GetAdminAuth(c)
		if err != nil {
			return err
		}
		c.Locals(coresession.AuthAdminKey.String(), admin)
		require.NotNil(t, adminmiddleware.GetAdminAuth(c))
		if err := checker(c); err != nil {
			return err
		}
		called++
		return c.SendStatus(http.StatusNoContent)
	})
	login, err := f.app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/login", nil))
	require.NoError(t, err)
	defer login.Body.Close()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, cfg.DomainURL+"/command", nil)
	r.Header.Set("Origin", cfg.DomainURL)
	r.Header.Set("X-Inertia", "true")
	r.Header.Set("X-Goadmin-Session-Maintenance", "1")
	for _, cookie := range login.Cookies() {
		r.AddCookie(cookie)
	}
	response, err := app.Test(r)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, 419, response.StatusCode)
	require.Equal(t, "1", response.Header.Get("X-Goadmin-Auth-Error"))
	require.Empty(t, response.Header.Get("Location"))
	require.Zero(t, called)
	var body sessionMaintenanceResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	require.Equal(t, "csrf_needs_sync", body.Code)
}

func TestSessionMaintenancePagePinsRenderedLogin(t *testing.T) {
	f := newRealtimeLogoutFixture(t)
	f.app.Get("/page-session", func(c fiber.Ctx) error {
		admin, err := f.service.GetAdminAuth(c)
		if err != nil {
			return err
		}
		c.Locals(coresession.AuthAdminKey.String(), admin)
		prop := sessionMaintenancePageProp(c, f.service)
		if admin != nil {
			snapshot, snapshotErr := f.service.MaintenanceSnapshot(c)
			require.NoError(t, snapshotErr)
			require.Equal(t, snapshot.Epoch, prop["sessionEpoch"])
			require.NotEqual(t, admin.AuthSessionID, prop["sessionEpoch"])
			require.NotEqual(t, admin.SessionID, prop["sessionEpoch"])
		}
		return c.JSON(prop)
	})
	page := func(cookies []*http.Cookie) map[string]any {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/page-session", nil)
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		response, err := f.app.Test(r)
		require.NoError(t, err)
		defer response.Body.Close()
		require.Equal(t, http.StatusOK, response.StatusCode)
		var prop map[string]any
		decoder := json.NewDecoder(response.Body)
		decoder.UseNumber()
		require.NoError(t, decoder.Decode(&prop))
		return prop
	}
	login := func() []*http.Cookie {
		response, err := f.app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/login", nil))
		require.NoError(t, err)
		defer response.Body.Close()
		require.Equal(t, http.StatusNoContent, response.StatusCode)
		return response.Cookies()
	}
	require.Empty(t, page(nil), "anonymous pages retain legacy mode")
	first := login()
	pinned := page(first)
	require.Equal(t, json.Number("1"), pinned["version"])
	require.NotEmpty(t, pinned["sessionEpoch"])
	require.Equal(t, pinned, page(first), "ordinary renders retain their login fence")
	require.NotEqual(t, pinned["sessionEpoch"], page(login())["sessionEpoch"], "another login of the same admin has a new fence")
}
