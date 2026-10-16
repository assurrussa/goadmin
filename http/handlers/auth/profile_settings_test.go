package auth //nolint:testpackage // verifies the private canonical-to-page props boundary

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	authmocks "github.com/assurrussa/goadmin/http/handlers/auth/mocks"
	"github.com/assurrussa/goadmin/infrastructure/core/session"
	"github.com/assurrussa/goadmin/models"
)

const profileFlashSuccess = "success"

func TestProfilePendingEmailChangeProps(t *testing.T) {
	t.Parallel()

	expiresAt := time.Date(2026, time.September, 29, 12, 34, 56, 0, time.UTC)
	pending := &goauth.PendingEmailChange{
		ID: "internal-request-id", SubjectID: goauth.NewSubjectID(),
		NewDisplayValue: "next@example.test", Attempts: 1, MaxAttempts: 5,
		CreatedAt: expiresAt.Add(-15 * time.Minute), ExpiresAt: expiresAt,
	}
	props := profilePendingEmailChange(pending)
	encoded, err := json.Marshal(map[string]any{"pendingEmailChange": props})
	require.NoError(t, err)

	// Match the exact shape consumed by SettingsPage.vue; internal canonical
	// fields must not leak through Inertia serialization.
	require.JSONEq(t, `{"pendingEmailChange":{"newEmail":"next@example.test","expiresAt":"2026-09-29T12:34:56Z"}}`, string(encoded))
	require.Equal(t, pending.NewDisplayValue, props.NewEmail)
	require.Equal(t, expiresAt, props.ExpiresAt)
}

func TestProfilePendingEmailChangePropsAbsent(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(map[string]any{"pendingEmailChange": profilePendingEmailChange(nil)})
	require.NoError(t, err)
	require.JSONEq(t, `{"pendingEmailChange":null}`, string(encoded))
}

func TestProfileEmailRequestUsesPasswordAndAuthenticatedSubject(t *testing.T) {
	const jsonContentType = "application/json"
	for _, tc := range []struct {
		name, password, contentType, body string
		result                            error
	}{
		{
			name: "json", password: "space preserved password ", contentType: jsonContentType,
			body: `{"currentPassword":"space preserved password ","email":"next@example.test","subjectID":"attacker"}`,
		},
		{
			name: "form", password: "form-password", contentType: "application/x-www-form-urlencoded",
			body: "email=next%40example.test&currentPassword=form-password&subjectID=attacker",
		},
		{"missing", "", jsonContentType, `{"email":"next@example.test"}`, goauth.ErrCurrentPasswordInvalid},
		{
			name: "wrong", password: "wrong", contentType: jsonContentType,
			body: `{"email":"next@example.test","currentPassword":"wrong"}`, result: goauth.ErrCurrentPasswordInvalid,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			appTest := adminappt.NewAppTest(t)
			account := authmocks.NewMockadminAccountService(appTest.Ctrl)
			subjectID := goauth.NewSubjectID()
			account.EXPECT().RequestEmailChangeWithPassword(gomock.Any(), subjectID, tc.password, "next@example.test").Return(tc.result)
			handler := NewHandler(HandlerOptions{AdminApp: appTest.App, AdminAccountService: account})
			app := fiber.New()
			app.Post("/request", func(c fiber.Ctx) error {
				c.Locals(session.AuthAdminKey.String(), &models.SessionAdmin{ID: 42, SubjectID: subjectID})
				return handler.PostProfileEmailRequest(c)
			})
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/request", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			req.Header.Set("Referer", "/auth/profile/settings")
			response, err := app.Test(req)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, http.StatusFound, response.StatusCode)
		})
	}
}

func TestProfilePasswordValidationUsesPageFieldKeys(t *testing.T) {
	for _, tc := range []struct{ name, body, key string }{
		{"required current", `{"password":"new","passwordConfirmation":"new"}`, "currentPassword"},
		{"required password", `{"currentPassword":"current"}`, "password"},
		{"required confirmation", `{"currentPassword":"current","password":"new"}`, "passwordConfirmation"},
		{"mismatch", `{"currentPassword":"current","password":"new","passwordConfirmation":"other"}`, "passwordConfirmation"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := adminappt.NewAppTest(t)
			handler := NewHandler(HandlerOptions{
				AdminApp: harness.App, AdminAccountService: authmocks.NewMockadminAccountService(harness.Ctrl),
			})
			var props map[string]any
			app := fiber.New()
			app.Post("/password", func(c fiber.Ctx) error {
				err := handler.PostProfileChangePassword(c)
				props = harness.InertiaManager.State(c).Props
				return err
			})
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/password", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Referer", "/auth/profile/settings")
			response, err := app.Test(req)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, http.StatusFound, response.StatusCode)
			require.Contains(t, props["errors"], tc.key)
			require.NotContains(t, props["errors"], "confirmPassword")
		})
	}
}

func TestProfilePasswordChangeCanonicalResult(t *testing.T) {
	for _, tc := range []struct {
		name    string
		result  error
		message string
	}{
		{"wrong current", goauth.ErrCurrentPasswordInvalid, "Неверный текущий пароль."},
		{"unchanged", goauth.ErrPasswordUnchanged, "Новый пароль совпадает с текущим."},
		{"policy", goauth.ErrInvalidPassword, "Новый пароль не соответствует политике безопасности."},
		{"common", goauth.ErrCommonPassword, "Новый пароль не соответствует политике безопасности."},
		{profileFlashSuccess, nil, "Пароль обновлён. Войдите снова."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := adminappt.NewAppTest(t)
			subjectID := goauth.NewSubjectID()
			account := authmocks.NewMockadminAccountService(harness.Ctrl)
			actions := authmocks.NewMocksessionStoreActions(harness.Ctrl)
			account.EXPECT().ChangePassword(gomock.Any(), subjectID, " current ", "new-password").Return(tc.result)
			if tc.result == nil {
				actions.EXPECT().DelAdminAuth(gomock.Any()).Return(nil)
			}
			handler := NewHandler(HandlerOptions{AdminApp: harness.App, AdminAccountService: account, SessionStoreActions: actions})
			var props map[string]any
			app := fiber.New()
			app.Post("/password", func(c fiber.Ctx) error {
				c.Locals(session.AuthAdminKey.String(), &models.SessionAdmin{ID: 42, SubjectID: subjectID})
				err := handler.PostProfileChangePassword(c)
				props = harness.InertiaManager.State(c).Props
				return err
			})
			body := `{"currentPassword":" current ","password":"new-password",
 "passwordConfirmation":"new-password","subjectID":"attacker"}`
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/password", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Referer", "/auth/profile/settings")
			response, err := app.Test(req)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, http.StatusFound, response.StatusCode)
			key, target := "error", "/auth/profile/settings"
			if tc.result == nil {
				key, target = profileFlashSuccess, "/auth/login"
			}
			require.Equal(t, target, response.Header.Get("Location"))
			require.Equal(t, map[string]string{key: tc.message}, props["flash"])
		})
	}
}

func TestProfileSettingsPendingEmailCanonicalState(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mail    bool
		pending *goauth.PendingEmailChange
		result  error
	}{
		{name: "absent", mail: true, result: goauth.ErrEmailChangeNotFound},
		{name: "pending", mail: true, pending: &goauth.PendingEmailChange{
			NewDisplayValue: "next@example.test", ExpiresAt: time.Now().UTC().Round(0).Add(time.Hour),
		}},
		{name: "expired", mail: true, result: goauth.ErrEmailChangeNotFound},
		{name: "authmail absent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := adminappt.NewAppTest(t)
			harness.App.SetCapabilities(map[string]bool{"authmail": tc.mail})
			subjectID := goauth.NewSubjectID()
			repo := authmocks.NewMockadminRepo(harness.Ctrl)
			account := authmocks.NewMockadminAccountService(harness.Ctrl)
			repo.EXPECT().GetByID(gomock.Any(), int64(42)).Return(models.Admin{ID: 42, Email: "current@example.test"}, nil)
			if tc.mail {
				account.EXPECT().GetPendingEmailChange(gomock.Any(), subjectID).Return(tc.pending, tc.result)
			}
			handler := NewHandler(HandlerOptions{AdminApp: harness.App, AdminRepo: repo, AdminAccountService: account})
			app := fiber.New()
			app.Get("/settings", func(c fiber.Ctx) error {
				c.Locals(session.AuthAdminKey.String(), &models.SessionAdmin{ID: 42, SubjectID: subjectID})
				return handler.ProfileSettings(c)
			})
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/settings", nil)
			req.Header.Set("X-Inertia", "true")
			response, err := app.Test(req)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, http.StatusOK, response.StatusCode)
			var page struct {
				Props struct {
					Data    models.Admin             `json:"data"`
					Pending *pendingEmailChangeProps `json:"pendingEmailChange"`
				} `json:"props"`
			}
			require.NoError(t, json.NewDecoder(response.Body).Decode(&page))
			require.Equal(t, "current@example.test", page.Props.Data.Email)
			require.Equal(t, profilePendingEmailChange(tc.pending), page.Props.Pending)
		})
	}
}
