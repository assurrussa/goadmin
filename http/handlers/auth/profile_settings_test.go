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
