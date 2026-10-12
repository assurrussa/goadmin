//nolint:testpackage // Verifies private login routing and no-secret response boundaries.
package auth

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	"github.com/assurrussa/goadmin/models"
	"github.com/assurrussa/goadmin/services/adminservice"
)

type mfaPolicyHandlerActions struct {
	sessionStoreActions
	loginErr   error
	setupCalls int
}

func TestRequiredLoginDoesNotRedirectBackIntoMFA(t *testing.T) {
	handler := &Handler{}
	app := fiber.New()
	app.Get("/destination", func(c fiber.Ctx) error {
		return c.SendString(handler.buildPostLoginRedirect(c, 0))
	})
	for _, target := range []string{"/auth/mfa", "/auth/mfa/enrollment/confirm"} {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
			"/destination?redirectback="+url.QueryEscape(target), nil)
		response, err := app.Test(request)
		require.NoError(t, err)
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		require.Equal(t, redirectTestProfile, string(body))
	}
}

func (a *mfaPolicyHandlerActions) LoginAdmin(
	fiber.Ctx, string, string, bool,
) (models.Admin, goauth.Account, error) {
	return models.Admin{}, goauth.Account{}, a.loginErr
}

func (a *mfaPolicyHandlerActions) LocalMFASetupSuggested(fiber.Ctx) bool { return true }

func (a *mfaPolicyHandlerActions) PendingLocalMFA(fiber.Ctx) (adminservice.LocalMFAPending, error) {
	return adminservice.LocalMFAPending{
		ID: "pending", Kind: goauth.MFALoginKindEnrollment, ExpiresAt: time.Now().Add(time.Minute),
	}, nil
}

func (a *mfaPolicyHandlerActions) CompleteLocalAdmin(
	fiber.Ctx, string, string,
) (models.Admin, goauth.Account, error) {
	return models.Admin{}, goauth.Account{}, goauth.ErrInvalidToken
}

func (a *mfaPolicyHandlerActions) BeginLocalAdminEnrollment(
	fiber.Ctx, string,
) (adminservice.MFALoginEnrollmentView, error) {
	a.setupCalls++
	return adminservice.MFALoginEnrollmentView{
		MFAEnrollmentView: adminservice.MFAEnrollmentView{
			ID: "enrollment", Secret: "synthetic-secret", URL: "otpauth://totp/synthetic",
			ExpiresAt: time.Now().Add(time.Minute),
		},
		RecoveryCodes: []string{"synthetic-recovery"},
	}, nil
}

func (a *mfaPolicyHandlerActions) CompleteLocalAdminEnrollment(
	fiber.Ctx, string, string, string,
) (models.Admin, goauth.Account, error) {
	return models.Admin{}, goauth.Account{}, nil
}

func TestOptionalLoginKeepsOriginalRedirectAndShowsReminder(t *testing.T) {
	appTest := adminappt.NewAppTest(t)
	actions := &mfaPolicyHandlerActions{}
	handler := NewHandler(HandlerOptions{AdminApp: appTest.App, SessionStoreActions: actions})
	app := fiber.New()
	var props map[string]any
	app.Post("/auth/login", func(c fiber.Ctx) error {
		err := handler.PostLogin(c)
		props = appTest.InertiaManager.State(c).Props
		return err
	})
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		"/auth/login?redirectback=%2Freports%3Ftab%3Dactive",
		strings.NewReader(`{"email":"admin@example.test","password":"synthetic-password"}`))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusFound, response.StatusCode)
	require.Equal(t, "/reports?tab=active", response.Header.Get("Location"))
	require.Equal(t, map[string]string{
		"info": "Для дополнительной защиты включите двухфакторную проверку в настройках профиля.",
	}, props["flash"])
	require.Zero(t, actions.setupCalls)
}

func TestRequiredLoginRetainsOriginalRedirectAcrossSetup(t *testing.T) {
	appTest := adminappt.NewAppTest(t)
	actions := &mfaPolicyHandlerActions{loginErr: adminservice.ErrLocalMFARequired}
	handler := NewHandler(HandlerOptions{AdminApp: appTest.App, SessionStoreActions: actions})
	app := fiber.New()
	app.Post("/auth/login", handler.PostLogin)
	app.Post("/auth/mfa/enrollment/confirm", handler.PostLocalMFAEnrollmentConfirm)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		"/auth/login?redirectback=%2Freports%3Ftab%3Dactive",
		strings.NewReader(`{"email":"admin@example.test","password":"synthetic-password"}`))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request)
	require.NoError(t, err)
	require.Equal(t, "/auth/mfa?redirectback=%2Freports%3Ftab%3Dactive", response.Header.Get("Location"))
	require.NoError(t, response.Body.Close())
	request = httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		"/auth/mfa/enrollment/confirm",
		strings.NewReader(`{"challengeID":"pending","enrollmentID":"enrollment","code":"123456"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Referer", "https://admin.example.test/auth/mfa?redirectback=%2Freports%3Ftab%3Dactive")
	response, err = app.Test(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.JSONEq(t, `{"redirect":"/reports?tab=active"}`, string(body))
	require.NotContains(t, string(body), "recoveryCodes")
}

func TestRequiredSetupGETNeverCreatesOrExposesEnrollment(t *testing.T) {
	appTest := adminappt.NewAppTest(t)
	actions := &mfaPolicyHandlerActions{}
	handler := NewHandler(HandlerOptions{AdminApp: appTest.App, SessionStoreActions: actions})
	app := fiber.New()
	app.Get("/auth/mfa", handler.GetLocalMFA)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/auth/mfa", nil)
	request.Header.Set("X-Inertia", "true")
	response, err := app.Test(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Zero(t, actions.setupCalls)
	require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
	require.Equal(t, http.StatusOK, response.StatusCode)
	var page struct {
		Component string         `json:"component"`
		Props     map[string]any `json:"props"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&page))
	require.Equal(t, "auth/MFAEnrollmentPage", page.Component)
	require.Equal(t, "pending", page.Props["challengeID"])
	require.NotContains(t, page.Props, "secret")
	require.NotContains(t, page.Props, "recoveryCodes")
}

func TestRequiredLoginRedirectNormalizationIsStable(t *testing.T) {
	for _, tc := range []struct {
		name   string
		query  string
		target string
	}{
		{
			name:  "nested encoded tab is not decoded across MFA",
			query: "redirectback=%2F%252509%2Fevil.example", target: "/%2509/evil.example",
		},
		{
			name: "decoded tab rejected", query: "redirectback=%2F%2509%2Fevil.example", target: redirectTestProfile,
		},
		{
			name:   "escaped local path query and fragment retained",
			query:  "redirectback=" + url.QueryEscape("/reports/a%3Fb%23c?tab=active#row%20one"),
			target: "/reports/a%3Fb%23c?tab=active#row%20one",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			appTest := adminappt.NewAppTest(t)
			appTest.MockRoleService.EXPECT().IsSuperAdmin(gomock.Any(), int64(0)).Return(false).AnyTimes()
			appTest.MockRoleService.EXPECT().AdminCan(gomock.Any(), int64(0), gomock.Any(), gomock.Any()).
				Return(false).AnyTimes()
			actions := &mfaPolicyHandlerActions{loginErr: adminservice.ErrLocalMFARequired}
			handler := NewHandler(HandlerOptions{AdminApp: appTest.App, SessionStoreActions: actions})
			app := fiber.New()
			app.Post("/auth/login", handler.PostLogin)
			app.Post("/auth/mfa/enrollment/confirm", handler.PostLocalMFAEnrollmentConfirm)
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
				"/auth/login?"+tc.query,
				strings.NewReader(`{"email":"admin@example.test","password":"synthetic-password"}`))
			request.Header.Set("Content-Type", "application/json")
			response, err := app.Test(request)
			require.NoError(t, err)
			require.Equal(t, http.StatusFound, response.StatusCode)
			mfaURL := response.Header.Get("Location")
			require.True(t, strings.HasPrefix(mfaURL, "/auth/mfa"))
			require.NoError(t, response.Body.Close())
			request = httptest.NewRequestWithContext(t.Context(), http.MethodPost,
				"/auth/mfa/enrollment/confirm",
				strings.NewReader(`{"challengeID":"pending","enrollmentID":"enrollment","code":"123456"}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Referer", redirectTestAdminURL+mfaURL)
			response, err = app.Test(request)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, http.StatusOK, response.StatusCode)
			var result map[string]string
			require.NoError(t, json.NewDecoder(response.Body).Decode(&result))
			require.Equal(t, tc.target, result["redirect"])
		})
	}
}
