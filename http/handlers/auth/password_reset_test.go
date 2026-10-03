package auth //nolint:testpackage // checks the handler's flash props without sending mail

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	authmocks "github.com/assurrussa/goadmin/http/handlers/auth/mocks"
)

func TestPasswordResetRequestAcknowledgesOnlyProcessing(t *testing.T) {
	t.Parallel()

	for _, email := range []string{"eligible@example.test", "missing@example.test"} {
		t.Run(email, func(t *testing.T) {
			t.Parallel()
			appTest := adminappt.NewAppTest(t)
			account := authmocks.NewMockadminAccountService(appTest.Ctrl)
			// Both accepted and suppressed requests have the same nil result;
			// neither supplies queue or provider-acceptance evidence to the handler.
			account.EXPECT().RequestPasswordReset(gomock.Any(), email).Return(nil)
			handler := NewHandler(HandlerOptions{AdminApp: appTest.App, AdminAccountService: account})
			props := resetRequestProps(t, appTest, handler, email)
			require.Equal(t, map[string]string{
				"success": "Запрос принят. Отправка письма ещё не подтверждена. Проверьте почту перед повторной попыткой.",
			}, props["flash"])
			require.NotContains(t, props["flash"], email)
		})
	}
}

func TestPasswordResetRequestFailureNeverClaimsSuccess(t *testing.T) {
	t.Parallel()

	for _, missingService := range []bool{false, true} {
		t.Run(map[bool]string{false: "service error", true: "service missing"}[missingService], func(t *testing.T) {
			t.Parallel()

			appTest := adminappt.NewAppTest(t)
			opts := HandlerOptions{AdminApp: appTest.App}
			if !missingService {
				account := authmocks.NewMockadminAccountService(appTest.Ctrl)
				account.EXPECT().RequestPasswordReset(gomock.Any(), "fixture@example.test").
					Return(errors.New("private provider diagnostic"))
				opts.AdminAccountService = account
			}
			props := resetRequestProps(t, appTest, NewHandler(opts), "fixture@example.test")
			require.Equal(t, map[string]string{
				"error": "Что-то пошло не так, попробуйте повторить операцию позже! Код: 13344",
			}, props["flash"])
		})
	}
}

func resetRequestProps(t *testing.T, appTest *adminappt.App, handler *Handler, email string) map[string]any {
	t.Helper()

	var props map[string]any
	app := fiber.New()
	app.Post("/auth/forgot-password", func(c fiber.Ctx) error {
		err := handler.PostForgotPassword(c)
		props = appTest.InertiaManager.State(c).Props
		return err
	})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/auth/forgot-password",
		strings.NewReader(`{"email":"`+email+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "/auth/forgot-password")
	response, err := app.Test(req)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusFound, response.StatusCode)
	require.Equal(t, "/auth/forgot-password", response.Header.Get("Location"))
	return props
}
