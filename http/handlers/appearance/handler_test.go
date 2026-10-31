package appearance //nolint:testpackage // Exercises private strict decoder and sanitized API boundary.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	"github.com/assurrussa/goadmin/internal/adminpreferences"
	core "github.com/assurrussa/goadmin/internal/appearance"
	"github.com/assurrussa/goadmin/internal/auth"
)

func TestAppearanceRegistrationDoesNotProtectLaterAnonymousLogin(t *testing.T) {
	harness := adminappt.NewAppTest(t)
	harness.ExpertGuard(auth.PermissionDomainBranding, auth.PermissionActionRead).AnyTimes()
	harness.ExpertGuard(auth.PermissionDomainBranding, auth.PermissionActionUpdate).AnyTimes()
	app := fiber.New()
	h := NewHandler(harness.App, core.NewService(nil, nil, nil))
	h.RegisterRoutes(app)
	h.RegisterGroupRoutes(app)
	app.Get("/auth/login", func(c fiber.Ctx) error { return c.SendString("login") })
	response, err := app.Test(httptest.NewRequestWithContext(context.Background(), "GET", "/auth/login", nil))
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, 200, response.StatusCode)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, "login", string(body))
	for _, path := range []string{"/auth/profile/appearance", "/auth/branding"} {
		res, e := app.Test(httptest.NewRequestWithContext(context.Background(), "PATCH", path, strings.NewReader(`{}`)))
		require.NoError(t, e)
		require.NotEqual(t, 200, res.StatusCode)
		require.NoError(t, res.Body.Close())
	}
}

func TestAppearanceAPISanitizesInternalErrorsAndPreservesConflict(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
	}{
		{errors.New("database password secret"), 500},
		{adminpreferences.ErrConflict, 409},
		{core.ErrInvalid, 400},
		{fiber.ErrForbidden, 403},
	} {
		app := fiber.New()
		app.Get("/", api(func(fiber.Ctx) error { return test.err }))
		res, err := app.Test(httptest.NewRequestWithContext(context.Background(), "GET", "/", nil))
		require.NoError(t, err)
		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())
		require.Equal(t, test.status, res.StatusCode)
		require.Contains(t, res.Header.Get("Content-Type"), "application/json")
		require.Equal(t, "no-store", res.Header.Get("Cache-Control"))
		require.NotContains(t, string(body), "secret")
	}
}

func TestAppearanceStrictDocumentInput(t *testing.T) {
	for _, body := range []string{
		`{"version":0,"owner":"other","payload":{}}`,
		`{"version":0} {}`, `{"version":-1,`, strings.Repeat("x", 4100),
	} {
		app := fiber.New()
		app.Patch("/", api(func(c fiber.Ctx) error { var req core.PersonalRequest; return decode(c, &req, 4096) }))
		res, err := app.Test(httptest.NewRequestWithContext(context.Background(), "PATCH", "/", strings.NewReader(body)))
		require.NoError(t, err)
		require.True(t, res.StatusCode == http.StatusBadRequest || res.StatusCode == http.StatusRequestEntityTooLarge)
		require.NoError(t, res.Body.Close())
	}
}
