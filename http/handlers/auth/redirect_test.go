package auth //nolint:testpackage // verifies the private post-login destination policy

import (
	"context"
	"io"
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
	"github.com/assurrussa/goadmin/infrastructure/core/menu"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
)

const (
	redirectTestAdminURL = "https://admin.example.test"
	redirectTestProfile  = "/auth/profile"
)

func TestPostLoginRedirectUsesFirstPermittedPage(t *testing.T) {
	const queuesPath = "/queues"
	for _, tc := range []struct {
		name        string
		query       string
		allowHome   bool
		allowQueues bool
		want        string
	}{
		{name: "restricted role", allowQueues: true, want: queuesPath},
		{name: "no menu permission", want: redirectTestProfile},
		{name: "valid deep link", query: "?redirectback=/auth/profile/settings", want: "/auth/profile/settings"},
		{name: "external redirect rejected", query: "?redirectback=https://evil.example/", allowQueues: true, want: queuesPath},
		{
			name: "same host deep link", query: "?redirectback=https://admin.example.test/queues?filter=active",
			allowQueues: true, want: "/queues?filter=active",
		},
		{
			name: "same host doubled slash rejected", query: "?redirectback=https://admin.example.test//evil.example/path",
			allowQueues: true, want: queuesPath,
		},
		{
			name: "same host encoded doubled slash rejected", query: "?redirectback=https://admin.example.test/%2Fevil.example/path",
			allowQueues: true, want: queuesPath,
		},
		{name: "backslash redirect rejected", query: "?redirectback=/%5Cevil.example", allowQueues: true, want: queuesPath},
		{name: "encoded backslash rejected", query: "?redirectback=/%255Cevil.example", allowQueues: true, want: queuesPath},
		{name: "forbidden home falls back", query: "?redirectback=/", allowQueues: true, want: queuesPath},
		{name: "permitted home retained", query: "?redirectback=%2F%3Ftab%3Drecent", allowHome: true, want: "/?tab=recent"},
		{name: "login loop avoided", query: "?redirectback=/auth/login", allowQueues: true, want: queuesPath},
	} {
		t.Run(tc.name, func(t *testing.T) {
			appTest := adminappt.NewAppTest(t)
			appTest.MockRoleService.EXPECT().IsSuperAdmin(gomock.Any(), int64(42)).Return(false).AnyTimes()
			appTest.MockRoleService.EXPECT().AdminCan(gomock.Any(), int64(42), gomock.Any(), gomock.Any()).
				DoAndReturn(func(
					_ context.Context, _ int64,
					domain integrationroles.PermissionDomain,
					_ integrationroles.PermissionAction,
				) bool {
					return tc.allowHome && domain == integrationroles.PermissionDomainDashboard ||
						tc.allowQueues && domain == integrationroles.PermissionDomainQueues
				}).AnyTimes()

			handler := NewHandler(HandlerOptions{
				AdminApp: appTest.App, AdminDomainURL: redirectTestAdminURL,
				AdminMenu: menu.ModuleMenu(map[string]bool{"queues": true}),
			})
			app := fiber.New()
			app.Get("/auth/login", func(c fiber.Ctx) error {
				return c.SendString(handler.buildPostLoginRedirect(c, 42))
			})

			resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/auth/login"+tc.query, nil))
			require.NoError(t, err)
			defer func() { require.NoError(t, resp.Body.Close()) }()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, tc.want, string(body))
		})
	}
}

func TestPostResetPasswordRejectsMismatchedConfirmation(t *testing.T) {
	appTest := adminappt.NewAppTest(t)
	account := authmocks.NewMockadminAccountService(appTest.Ctrl)
	account.EXPECT().PerformPasswordReset(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	handler := NewHandler(HandlerOptions{AdminApp: appTest.App, AdminAccountService: account})
	app := fiber.New()
	app.Post("/auth/reset-password", handler.PostResetPassword)

	body := `{"token":"valid-token","password":"first-password","passwordConfirmation":"other-password"}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/auth/reset-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "/auth/reset-password")
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusFound, resp.StatusCode)
}

func TestPostLoginRedirectUsesAnyPermissionMenu(t *testing.T) {
	const studentsPath = "/students"
	for _, action := range []integrationroles.PermissionAction{"", "read", "create", "update", "delete"} {
		t.Run(string(action), func(t *testing.T) {
			appTest := adminappt.NewAppTest(t)
			appTest.MockRoleService.EXPECT().IsSuperAdmin(gomock.Any(), int64(42)).Return(false).AnyTimes()
			appTest.MockRoleService.EXPECT().AdminCan(gomock.Any(), int64(42), gomock.Any(), gomock.Any()).DoAndReturn(func(
				_ context.Context, _ int64, domain integrationroles.PermissionDomain, candidate integrationroles.PermissionAction,
			) bool {
				return domain == "users" && candidate == action
			}).AnyTimes()
			handler := NewHandler(HandlerOptions{AdminApp: appTest.App, AdminMenu: menu.Menu{Sections: []menu.Section{{
				Key: "students", Items: []menu.Item{{Name: "Group", Children: []menu.Item{{
					Href: studentsPath, AnyPermissionKeys: []integrationroles.PermissionKey{
						integrationroles.NewPermissionKey("users", "read"), integrationroles.NewPermissionKey("users", "create"),
						integrationroles.NewPermissionKey("users", "update"), integrationroles.NewPermissionKey("users", "delete"),
					},
				}}}},
			}}}})
			app := fiber.New()
			app.Get("/auth/login", func(c fiber.Ctx) error { return c.SendString(handler.buildPostLoginRedirect(c, 42)) })
			resp, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/auth/login", nil))
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			want := studentsPath
			if action == "" {
				want = redirectTestProfile
			}
			require.Equal(t, want, string(body))
		})
	}
}

func TestNormalizeRedirectTargetRejectsDecodedControls(t *testing.T) {
	handler := &Handler{baseURL: redirectTestAdminURL}
	for _, encoded := range []string{"%00", "%09", "%0A", "%0D", "%1F", "%7F", "%C2%85"} {
		t.Run(encoded, func(t *testing.T) {
			for _, target := range []string{
				"/" + encoded + "/evil.example",
				"https://admin.example.test/" + encoded + "/evil.example",
				"/reports#" + encoded,
			} {
				require.Empty(t, handler.normalizeRedirectTarget(target), target)
			}
		})
	}
}

func TestNormalizeRedirectTargetPreservesEscapedDestinations(t *testing.T) {
	handler := &Handler{baseURL: redirectTestAdminURL}
	require.Equal(t, "/reports", handler.normalizeRedirectTarget("%2Freports"))
	for _, target := range []string{
		"/reports?tab=active#summary",
		"/reports/a%20b?next=%2Fqueues%3Fq%3Da%2520b#row%20one",
		"/reports/a%3Fb%23c%25?filter=active#part%23one",
		"/%2509/evil.example",
		"/%252F/evil.example",
		"/%255Cevil.example",
	} {
		t.Run(target, func(t *testing.T) {
			cleaned := handler.normalizeRedirectTarget(target)
			require.Equal(t, target, cleaned)
			require.Equal(t, cleaned, handler.normalizeRedirectTarget(cleaned))
			require.Equal(t, cleaned, handler.normalizeRedirectTarget(redirectTestAdminURL+target))
			parsed, err := url.Parse(cleaned)
			require.NoError(t, err)
			require.Empty(t, parsed.Host)
		})
	}
}

func TestPostLoginRedirectRejectsEncodedControlsWithoutPanic(t *testing.T) {
	handler := &Handler{}
	app := fiber.New()
	app.Get("/auth/login", func(c fiber.Ctx) error {
		return c.SendString(handler.buildPostLoginRedirect(c, 0))
	})
	for _, target := range []string{"/%09/evil.example", "/reports#%0A", "/reports%"} {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
			"/auth/login?redirectback="+url.QueryEscape(target), nil)
		response, err := app.Test(request)
		require.NoError(t, err)
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		require.Equal(t, http.StatusOK, response.StatusCode)
		require.Equal(t, redirectTestProfile, string(body))
	}
}
