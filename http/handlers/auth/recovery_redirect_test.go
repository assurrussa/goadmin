package auth //nolint:testpackage // verifies the configured recovery boundary

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	authmocks "github.com/assurrussa/goadmin/http/handlers/auth/mocks"
	"github.com/assurrussa/goadmin/internal/authrecovery"
)

type recoveryScopeTestKey struct{}

func TestConfiguredPasswordResetStaysOnRecoveryOrigin(t *testing.T) {
	for _, loginURL := range []string{"https://sso.example.test/sso/account", "/auth/login", ""} {
		t.Run(loginURL, func(t *testing.T) {
			appTest := adminappt.NewAppTest(t)
			account := authmocks.NewMockadminAccountService(appTest.Ctrl)
			committed := 0
			account.EXPECT().PerformPasswordReset(gomock.Any(), "synthetic-token", "Unique-Password-123").DoAndReturn(
				func(ctx context.Context, _, _ string) error {
					require.Equal(t, "central", ctx.Value(recoveryScopeTestKey{}))
					committed++
					return nil
				},
			)
			handler := NewHandler(HandlerOptions{AdminApp: appTest.App, AdminAccountService: account})
			handler.recovery = &authrecovery.Routes{
				ForgotPath: "/auth/sso/forgot-password", ResetPaths: []string{"/auth/sso/reset-password"}, LoginURL: loginURL,
				Context: func(ctx context.Context) (context.Context, error) {
					return context.WithValue(ctx, recoveryScopeTestKey{}, "central"), nil
				},
			}
			app := fiber.New()
			// Model the operator boundary: a committed reset must not subsequently
			// fail because a cross-origin HTTP Location escaped this admin listener.
			app.Use(func(c fiber.Ctx) error {
				if err := c.Next(); err != nil {
					return err
				}
				destination, err := url.Parse(string(c.Response().Header.Peek("Location")))
				if err != nil {
					return c.SendStatus(500)
				}
				if destination.IsAbs() && destination.Host != "admin.example.test" {
					return c.SendStatus(500)
				}
				return nil
			})
			var props map[string]any
			app.Post("/auth/sso/reset-password", func(c fiber.Ctx) error {
				err := handler.PostResetPassword(c)
				props = appTest.InertiaManager.State(c).Props
				return err
			})
			body := `{"token":"synthetic-token","password":"Unique-Password-123","passwordConfirmation":"Unique-Password-123"}`
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
				"https://admin.example.test/auth/sso/reset-password", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Referer", "https://admin.example.test/auth/sso/reset-password")
			response, err := app.Test(request)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, 1, committed)
			require.Equal(t, http.StatusFound, response.StatusCode)
			require.Equal(t, "/auth/sso/forgot-password", response.Header.Get("Location"))
			require.Equal(t, map[string]string{"success": "Успешный сброс пароля!"}, props["flash"])
		})
	}
}
