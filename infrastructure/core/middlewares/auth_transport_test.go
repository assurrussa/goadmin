//nolint:testpackage // verifies final HTTP responses, including the private auth error boundary.
package adminmiddleware

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	requestmiddleware "github.com/assurrussa/goadmin/infrastructure/fiber/middlewares"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
	"github.com/assurrussa/goadmin/services/adminservice"
)

func newAuthTransportTestApp() (*fiber.App, *goinertia.Inertia, *atomic.Int32) {
	inertia := goinertia.New("https://admin.test")
	listener := inertia.MiddlewareErrorListener()
	fallbackCalls := &atomic.Int32{}
	app := fiber.New(fiber.Config{ErrorHandler: func(c fiber.Ctx, err error) error {
		fallbackCalls.Add(1)
		return listener(c, err)
	}})
	// The production request logger invokes the configured error listener
	// itself. Include it so testing just an error's Code cannot hide a redirect.
	app.Use(requestmiddleware.NewRequestLogger(requestmiddleware.RequestLoggerConfig{Stream: io.Discard}))
	return app, inertia, fallbackCalls
}

func TestBrowserAuthStatusesSurviveInertiaErrorListener(t *testing.T) {
	const codeReauthRequired = "reauthentication_required"
	for _, tc := range []struct {
		name       string
		err        error
		status     int
		code       string
		retryAfter string
	}{
		{"invalid", goauth.ErrInvalidToken, http.StatusBadRequest, "invalid_browser_credentials", ""},
		{"revoked", goauth.ErrSessionRevoked, http.StatusUnauthorized, codeReauthRequired, ""},
		{"unknown", goauth.ErrOperationOutcomeUnknown, http.StatusUnauthorized, codeReauthRequired, ""},
		{
			"unknown-and-invalid",
			errors.Join(goauth.ErrOperationOutcomeUnknown, goauth.ErrInvalidToken),
			http.StatusUnauthorized,
			codeReauthRequired,
			"",
		},
		{"busy", adminservice.ErrRefreshInProgress, http.StatusServiceUnavailable, "authentication_retry_required", "1"},
		{"retry", adminservice.ErrRefreshRetryRequired, http.StatusServiceUnavailable, "authentication_retry_required", "1"},
		{"backend", errors.New("SECRET_BACKEND_DETAILS"), http.StatusServiceUnavailable, "authentication_unavailable", ""},
	} {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
			for _, inertiaRequest := range []bool{false, true} {
				t.Run(tc.name+"/"+method+"/inertia="+strconv.FormatBool(inertiaRequest), func(t *testing.T) {
					app, inertia, fallbackCalls := newAuthTransportTestApp()
					var mutations atomic.Int32
					// Like bootstrap, auth and anonymous guards precede Inertia.
					app.Use(func(c fiber.Ctx) error { return browserAuthFailure(c, tc.err) })
					app.Use(IsNotAuthAdminMiddleware())
					app.Use(inertia.Middleware())
					app.All("/protected", func(c fiber.Ctx) error {
						mutations.Add(1)
						return c.SendStatus(http.StatusNoContent)
					})
					req := httptest.NewRequestWithContext(t.Context(), method, "/protected", nil)
					req.Header.Set(fiber.HeaderReferer, "https://admin.test/protected")
					if inertiaRequest {
						req.Header.Set(goinertia.HeaderInertia, "true")
					}
					resp, err := app.Test(req)
					require.NoError(t, err)
					defer resp.Body.Close()
					require.Equal(t, tc.status, resp.StatusCode)
					require.Equal(t, "1", resp.Header.Get("X-Goadmin-Auth-Error"))
					require.Empty(t, resp.Header.Get(fiber.HeaderLocation))
					require.Empty(t, resp.Header.Get("X-Inertia-Location"))
					require.Empty(t, resp.Header.Get(goinertia.HeaderInertia))
					require.Empty(t, resp.Header.Values("Set-Cookie"), "failure must not save flash/session state")
					require.Equal(t, "no-store", resp.Header.Get(fiber.HeaderCacheControl))
					require.Equal(t, tc.retryAfter, resp.Header.Get(fiber.HeaderRetryAfter))
					require.Contains(t, resp.Header.Get(fiber.HeaderContentType), "application/json")
					body, err := io.ReadAll(resp.Body)
					require.NoError(t, err)
					require.NotContains(t, string(body), "SECRET_BACKEND_DETAILS")
					var problem browserAuthResponse
					require.NoError(t, json.Unmarshal(body, &problem))
					require.Equal(t, tc.status, problem.Status)
					require.Equal(t, tc.code, problem.Code)
					require.Zero(t, mutations.Load())
					require.Zero(t, fallbackCalls.Load(), "generic Inertia listener must not convert auth failures")
				})
			}
		}
	}
}

func TestAnonymousMutationsRemainUnauthorizedWithInertia(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			app, inertia, fallbackCalls := newAuthTransportTestApp()
			var mutations atomic.Int32
			app.Use(IsNotAuthAdminMiddleware())
			app.Use(inertia.Middleware())
			app.All("/protected", func(c fiber.Ctx) error {
				mutations.Add(1)
				return c.SendStatus(http.StatusNoContent)
			})
			req := httptest.NewRequestWithContext(t.Context(), method, "/protected", strings.NewReader("{}"))
			req.Header.Set(goinertia.HeaderInertia, "true")
			req.Header.Set(fiber.HeaderReferer, "https://admin.test/protected")
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
			require.Equal(t, "1", resp.Header.Get("X-Goadmin-Auth-Error"))
			require.Empty(t, resp.Header.Get(fiber.HeaderLocation))
			require.Equal(t, "no-store", resp.Header.Get(fiber.HeaderCacheControl))
			require.Zero(t, mutations.Load())
			require.Zero(t, fallbackCalls.Load())
		})
	}
}

func TestAuthMiddlewareUnavailableDoesNotRedirectSafeRequests(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			app, inertia, fallbackCalls := newAuthTransportTestApp()
			app.Use(AuthAdminMiddleware(nil))
			app.Use(IsNotAuthAdminMiddleware())
			app.Use(inertia.Middleware())
			app.All("/protected", func(c fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })
			req := httptest.NewRequestWithContext(t.Context(), method, "/protected", nil)
			req.Header.Set(goinertia.HeaderInertia, "true")
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
			require.Equal(t, "1", resp.Header.Get("X-Goadmin-Auth-Error"))
			require.Empty(t, resp.Header.Get(fiber.HeaderLocation))
			require.Zero(t, fallbackCalls.Load())
		})
	}
}
