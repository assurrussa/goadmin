package bootstrap //nolint:testpackage // test package

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	logger "github.com/assurrussa/gologger"
	"github.com/gofiber/fiber/v3"
	fibersession "github.com/gofiber/fiber/v3/middleware/session"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	adminconfig "github.com/assurrussa/goadmin/config"
	coresession "github.com/assurrussa/goadmin/infrastructure/core/session"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
	csrfservice "github.com/assurrussa/goadmin/internal/csrf"
	identity "github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
	"github.com/assurrussa/goadmin/services/adminservice"
)

const (
	testAdminDomain = "admin.localhost"
	testCSRFToken   = "csrf_token"
	testEnvLocal    = "local"
)

func newTestCSRFService(t *testing.T) *csrfservice.Service {
	t.Helper()

	svc, err := csrfservice.New(csrfservice.NewOptions(
		testAdminDomain,
		"test-secret-key-1234567890123456",
		[]string{"https://" + testAdminDomain},
		logger.Discard(),
		csrfservice.WithTokenTTL(time.Hour),
	))
	require.NoError(t, err)

	return svc
}

func newTestSessionStore(t *testing.T) *fibersession.Store {
	t.Helper()

	return fibersession.NewStore(fibersession.Config{
		CookiePath:     "/",
		CookieDomain:   testAdminDomain,
		CookieSameSite: "Lax",
		CookieSecure:   false,
	})
}

func TestCSRFTokenProvider_GuestGeneratesCookieWithPathAndLax(t *testing.T) {
	csrfSvc := newTestCSRFService(t)
	sessStore := newTestSessionStore(t)
	adminAuthSvc := adminservice.NewService(sessStore, nil)
	cfg := adminconfig.Config{
		Domain:        testAdminDomain,
		DomainURL:     "https://" + testAdminDomain,
		CSRFTokenName: testCSRFToken,
		CSRFTokenTTL:  time.Hour,
		Env:           testEnvLocal,
	}

	provider := newCSRFTokenProvider(adminAuthSvc, sessStore, csrfSvc, cfg)
	require.NotNil(t, provider)

	checker := newCheckCSRFTokenProvider(sessStore, csrfSvc, cfg)
	require.NotNil(t, checker)

	app := fiber.New(fiber.Config{
		ErrorHandler: func(c fiber.Ctx, err error) error {
			var inrErr *goinertia.Error
			if errors.As(err, &inrErr) {
				return c.Status(inrErr.Code).SendString(inrErr.Message)
			}
			return fiber.DefaultErrorHandler(c, err)
		},
	})

	var token string
	app.Get("/auth/register", func(c fiber.Ctx) error {
		var err error
		token, err = provider(c)
		if err != nil {
			return err
		}
		return c.SendStatus(fiber.StatusOK)
	})

	app.Post("/auth/register", func(c fiber.Ctx) error {
		if err := checker(c); err != nil {
			return err
		}
		return c.SendString("ok")
	})

	// 1. GET /auth/register
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/auth/register", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, fiber.StatusOK, resp.StatusCode)
	require.NotEmpty(t, token)

	// Check response Set-Cookie headers
	setCookies := resp.Header.Values("Set-Cookie")
	require.NotEmpty(t, setCookies)

	var csrfCookieVal, sessCookieVal string
	for _, sc := range setCookies {
		lower := strings.ToLower(sc)
		if strings.HasPrefix(sc, "csrf_token=") {
			require.Contains(t, lower, "path=/")
			require.Contains(t, lower, "samesite=lax")
			parts := strings.Split(sc, ";")
			csrfCookieVal = strings.TrimPrefix(parts[0], "csrf_token=")
		}
		if strings.HasPrefix(sc, "session_id=") {
			parts := strings.Split(sc, ";")
			sessCookieVal = strings.TrimPrefix(parts[0], "session_id=")
		}
	}
	require.NotEmpty(t, csrfCookieVal)
	require.NotEmpty(t, sessCookieVal)
	require.Equal(t, token, csrfCookieVal)
	claims, err := csrfSvc.ExtractClaims(token)
	require.NoError(t, err)
	require.Equal(t, csrfSessionBinding(sessCookieVal), claims.SessionID)
	require.NotEqual(t, sessCookieVal, claims.SessionID)

	// 2. POST /auth/register with matching CSRF cookie, session cookie, and X-CSRF-Token header
	reqPost := httptest.NewRequestWithContext(
		context.Background(),
		fiber.MethodPost,
		"/auth/register",
		strings.NewReader(`{}`),
	)
	reqPost.Header.Set("Content-Type", "application/json")
	reqPost.Header.Set("X-CSRF-Token", token)
	reqPost.Header.Set("Origin", "https://"+testAdminDomain)
	reqPost.Header.Set("Cookie", "session_id="+sessCookieVal+"; csrf_token="+csrfCookieVal)

	respPost, err := app.Test(reqPost)
	require.NoError(t, err)
	defer respPost.Body.Close()
	require.Equal(t, fiber.StatusOK, respPost.StatusCode)

	// 3. POST /auth/register with mismatched X-CSRF-Token header
	reqBadPost := httptest.NewRequestWithContext(
		context.Background(),
		fiber.MethodPost,
		"/auth/register",
		strings.NewReader(`{}`),
	)
	reqBadPost.Header.Set("Content-Type", "application/json")
	reqBadPost.Header.Set("X-CSRF-Token", "bad-token")
	reqBadPost.Header.Set("Origin", "https://"+testAdminDomain)
	reqBadPost.Header.Set("Cookie", "session_id="+sessCookieVal+"; csrf_token="+csrfCookieVal)

	respBadPost, err := app.Test(reqBadPost)
	require.NoError(t, err)
	defer respBadPost.Body.Close()
	require.Equal(t, 419, respBadPost.StatusCode)
}

func TestCSRFTokenProvider_AuthenticatedAdmin(t *testing.T) {
	csrfSvc := newTestCSRFService(t)
	sessStore := newTestSessionStore(t)
	adminAuthSvc := adminservice.NewService(sessStore, nil)
	cfg := adminconfig.Config{
		Domain:        testAdminDomain,
		DomainURL:     "https://" + testAdminDomain,
		CSRFTokenName: testCSRFToken,
		CSRFTokenTTL:  time.Hour,
		Env:           testEnvLocal,
	}

	provider := newCSRFTokenProvider(adminAuthSvc, sessStore, csrfSvc, cfg)
	require.NotNil(t, provider)

	checker := newCheckCSRFTokenProvider(sessStore, csrfSvc, cfg)
	require.NotNil(t, checker)

	adminUUID := identity.UserID(uuid.New())
	adminSession := &models.SessionAdmin{
		ID:    1,
		UUID:  adminUUID,
		Email: "admin@example.com",
	}

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals(coresession.AuthAdminKey.String(), adminSession)
		return c.Next()
	})

	var token string
	app.Get("/profile/settings", func(c fiber.Ctx) error {
		var err error
		token, err = provider(c)
		if err != nil {
			return err
		}
		return c.SendStatus(fiber.StatusOK)
	})

	app.Post("/profile/settings/password", func(c fiber.Ctx) error {
		if err := checker(c); err != nil {
			return err
		}
		return c.SendString("ok")
	})

	// 1. GET /profile/settings
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/profile/settings", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, fiber.StatusOK, resp.StatusCode)

	claims, err := csrfSvc.ExtractClaims(token)
	require.NoError(t, err)
	require.Equal(t, adminUUID.String(), claims.UserID.String())

	var csrfCookieVal, sessCookieVal string
	for _, sc := range resp.Header.Values("Set-Cookie") {
		if strings.HasPrefix(sc, "csrf_token=") {
			parts := strings.Split(sc, ";")
			csrfCookieVal = strings.TrimPrefix(parts[0], "csrf_token=")
		}
		if strings.HasPrefix(sc, "session_id=") {
			parts := strings.Split(sc, ";")
			sessCookieVal = strings.TrimPrefix(parts[0], "session_id=")
		}
	}

	// 2. POST /profile/settings/password
	reqPost := httptest.NewRequestWithContext(
		context.Background(),
		fiber.MethodPost,
		"/profile/settings/password",
		strings.NewReader(`{}`),
	)
	reqPost.Header.Set("Content-Type", "application/json")
	reqPost.Header.Set("X-CSRF-Token", token)
	reqPost.Header.Set("Origin", "https://"+testAdminDomain)
	reqPost.Header.Set("Cookie", "session_id="+sessCookieVal+"; csrf_token="+csrfCookieVal)

	respPost, err := app.Test(reqPost)
	require.NoError(t, err)
	defer respPost.Body.Close()
	require.Equal(t, fiber.StatusOK, respPost.StatusCode)
}

func TestCSRFTokenProvider_ReusesExistingValidToken(t *testing.T) {
	csrfSvc := newTestCSRFService(t)
	sessStore := newTestSessionStore(t)
	adminAuthSvc := adminservice.NewService(sessStore, nil)
	cfg := adminconfig.Config{
		Domain:        testAdminDomain,
		DomainURL:     "https://" + testAdminDomain,
		CSRFTokenName: testCSRFToken,
		CSRFTokenTTL:  time.Hour,
		Env:           testEnvLocal,
	}

	provider := newCSRFTokenProvider(adminAuthSvc, sessStore, csrfSvc, cfg)
	require.NotNil(t, provider)

	app := fiber.New()
	var tokens []string
	app.Get("/test", func(c fiber.Ctx) error {
		tok, err := provider(c)
		if err != nil {
			return err
		}
		tokens = append(tokens, tok)
		return c.SendStatus(fiber.StatusOK)
	})

	// First request: generates token
	req1 := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/test", nil)
	resp1, err := app.Test(req1)
	require.NoError(t, err)
	defer resp1.Body.Close()
	require.Equal(t, fiber.StatusOK, resp1.StatusCode)
	require.Len(t, tokens, 1)

	var csrfCookieVal, sessCookieVal string
	for _, sc := range resp1.Header.Values("Set-Cookie") {
		if strings.HasPrefix(sc, "csrf_token=") {
			parts := strings.Split(sc, ";")
			csrfCookieVal = strings.TrimPrefix(parts[0], "csrf_token=")
		}
		if strings.HasPrefix(sc, "session_id=") {
			parts := strings.Split(sc, ";")
			sessCookieVal = strings.TrimPrefix(parts[0], "session_id=")
		}
	}

	// Second request with existing cookies: reuses same token
	req2 := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/test", nil)
	req2.Header.Set("Cookie", "session_id="+sessCookieVal+"; csrf_token="+csrfCookieVal)
	resp2, err := app.Test(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()
	require.Equal(t, fiber.StatusOK, resp2.StatusCode)
	require.Len(t, tokens, 2)
	require.Equal(t, tokens[0], tokens[1])
}

func TestCSRFTokenChecker_MissingOrMismatched(t *testing.T) {
	csrfSvc := newTestCSRFService(t)
	sessStore := newTestSessionStore(t)
	cfg := adminconfig.Config{
		Domain:        testAdminDomain,
		DomainURL:     "https://" + testAdminDomain,
		CSRFTokenName: testCSRFToken,
		CSRFTokenTTL:  time.Hour,
		Env:           testEnvLocal,
	}

	checker := newCheckCSRFTokenProvider(sessStore, csrfSvc, cfg)
	require.NotNil(t, checker)

	app := fiber.New(fiber.Config{
		ErrorHandler: func(c fiber.Ctx, err error) error {
			var inrErr *goinertia.Error
			if errors.As(err, &inrErr) {
				return c.Status(inrErr.Code).SendString(inrErr.Message)
			}
			return fiber.DefaultErrorHandler(c, err)
		},
	})

	app.Post("/check", func(c fiber.Ctx) error {
		if err := checker(c); err != nil {
			return err
		}
		return c.SendString("ok")
	})

	// Missing header and cookie -> 419
	req1 := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/check", nil)
	resp1, err := app.Test(req1)
	require.NoError(t, err)
	defer resp1.Body.Close()
	require.Equal(t, 419, resp1.StatusCode)

	// Missing cookie -> 419
	req2 := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/check", nil)
	req2.Header.Set("X-CSRF-Token", "some-token")
	req2.Header.Set("Origin", "https://"+testAdminDomain)
	resp2, err := app.Test(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()
	require.Equal(t, 419, resp2.StatusCode)

	// Missing header -> 419
	req3 := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/check", nil)
	req3.Header.Set("Cookie", "csrf_token=some-token")
	resp3, err := app.Test(req3)
	require.NoError(t, err)
	defer resp3.Body.Close()
	require.Equal(t, 419, resp3.StatusCode)
}
