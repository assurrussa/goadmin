package bootstrap //nolint:testpackage // Exercises request middleware and server-owned session state.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/adminapp"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/models"
)

type previewOwner struct {
	adminapp.RolesService
	owner       bool
	allowUpdate bool
}

func (s *previewOwner) IsSuperAdmin(context.Context, int64) bool { return s.owner }
func (s *previewOwner) AdminGuardCheck(ctx context.Context, _ int64, key integrationroles.PermissionKey,
	_ ...integrationroles.PermissionGuardOption,
) bool {
	return s.allowUpdate && integrationroles.RolePreviewAllows(ctx, key)
}

type previewRoles struct{ RolesService }

const previewOtherPath = "/other"

func (*previewRoles) GetRole(context.Context, int64) (*integrationroles.Role, error) {
	return &integrationroles.Role{ID: 2, Name: "Viewer"}, nil
}

func (*previewRoles) ListRoles(context.Context, integrationroles.RoleFilter) ([]integrationroles.Role, error) {
	return []integrationroles.Role{{ID: 2, Name: "Viewer"}}, nil
}

func (*previewRoles) ListRolePermissions(context.Context, int64) ([]integrationroles.Permission, error) {
	return []integrationroles.Permission{{Domain: "users", Action: "read"}}, nil
}

func TestRolePreviewPersistsReadOnlyPolicyAndRequiresCurrentOwner(t *testing.T) {
	owner := &previewOwner{owner: true}
	deps := Dependencies{}
	deps.System.SessionStore = session.NewStore()
	deps.Repos.RolesService = owner
	deps.Repos.RolesManager = &previewRoles{}
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error { c.Locals("sess_admin", &models.SessionAdmin{ID: 1}); return c.Next() })
	app.Use(rolePreviewMiddleware(deps))
	writes := 0
	app.Post("/write", func(c fiber.Ctx) error { writes++; return c.SendStatus(204) })
	app.Get("/admins", func(c fiber.Ctx) error {
		if !integrationroles.RolePreviewAllows(c, integrationroles.NewPermissionKey("admins", "read")) {
			return fiber.ErrForbidden
		}
		return c.SendStatus(204)
	})
	app.Get("/users", func(c fiber.Ctx) error {
		if !integrationroles.RolePreviewAllows(c, integrationroles.NewPermissionKey("users", "read")) {
			return fiber.ErrForbidden
		}
		return c.SendStatus(204)
	})
	var cookies []*http.Cookie
	request := func(method, path, body string) int {
		req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		response, err := app.Test(req)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		if len(response.Cookies()) > 0 {
			cookies = response.Cookies()
		}
		return response.StatusCode
	}
	require.Equal(t, 200, request("POST", rolePreviewPath, `{"roleId":2}`))
	require.Equal(t, 403, request("POST", "/write", `{}`), "dropping preview parameters must not bypass read-only")
	require.Zero(t, writes)
	require.Equal(t, 403, request("GET", "/admins", ""), "direct URL denied by selected role")
	require.Equal(t, 204, request("GET", "/users", ""))
	owner.owner = false
	require.Equal(t, 403, request("POST", rolePreviewPath, `{"roleId":0}`))
	require.Equal(t, 403, request("GET", "/users", ""))
	owner.owner = true
	require.Equal(t, 200, request("POST", rolePreviewPath, `{"roleId":0}`))
	require.Equal(t, 204, request("POST", "/write", `{}`))
	require.Equal(t, 1, writes)
	owner.owner = false
	require.Equal(t, 403, request("POST", rolePreviewPath, `{"roleId":2}`))
}

func TestRolePreviewReportsSSOLinkDenialOnlyForJSONPost(t *testing.T) {
	deps := Dependencies{}
	deps.System.SessionStore = session.NewStore()
	owner := &previewOwner{owner: true}
	deps.Repos.RolesService = owner
	deps.Repos.RolesManager = &previewRoles{}
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error { c.Locals("sess_admin", &models.SessionAdmin{ID: 1}); return c.Next() })
	app.Use(rolePreviewMiddleware(deps))
	app.Post("/sso-link", func(c fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })
	app.Post(previewOtherPath, func(c fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })
	activate := httptest.NewRequestWithContext(t.Context(), http.MethodPost, rolePreviewPath,
		strings.NewReader(`{"roleId":2}`))
	activate.Header.Set("Content-Type", "application/json")
	active, err := app.Test(activate)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, active.StatusCode)
	require.NoError(t, active.Body.Close())
	for _, path := range []string{"/sso-link", previewOtherPath} {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, nil)
		request.Header.Set("Accept", "application/json")
		for _, cookie := range active.Cookies() {
			request.AddCookie(cookie)
		}
		response, err := app.Test(request)
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, response.StatusCode)
		if path == "/sso-link" {
			var body struct {
				Code string `json:"code"`
			}
			require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
			require.Equal(t, "role_preview_active", body.Code)
			require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
		} else {
			require.Equal(t, "text/plain; charset=utf-8", response.Header.Get("Content-Type"))
		}
		require.NoError(t, response.Body.Close())
	}
	owner.owner = false
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/sso-link", nil)
	request.Header.Set("Accept", "application/json")
	for _, cookie := range active.Cookies() {
		request.AddCookie(cookie)
	}
	response, err := app.Test(request)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, response.StatusCode)
	var body struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	require.Equal(t, "role_preview_invalid", body.Code)
	require.NoError(t, response.Body.Close())
}

func TestRolePreviewActionPresentationIntersectsCurrentGuard(t *testing.T) {
	for _, test := range []struct {
		name                                string
		selectedUpdate, actualUpdate, super bool
		want                                bool
	}{
		{name: "viewer", actualUpdate: true},
		{name: "selected editor without actual grant", selectedUpdate: true},
		{name: "selected editor", selectedUpdate: true, actualUpdate: true, want: true},
		{name: "selected super role", actualUpdate: true, super: true, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			deps := Dependencies{}
			deps.Repos.RolesService = &previewOwner{allowUpdate: test.actualUpdate}
			app := fiber.New()
			app.Get("/actions", func(c fiber.Ctx) error {
				c.Locals("sess_admin", &models.SessionAdmin{ID: 1})
				key := integrationroles.NewPermissionKey("admins", "update")
				c.SetContext(integrationroles.WithRolePreview(c.Context(), map[integrationroles.PermissionKey]bool{key: test.selectedUpdate}))
				if test.super {
					c.SetContext(integrationroles.WithSuperRolePreview(c.Context()))
				}
				actions := rolePreviewActionPresentation(c, deps)
				require.Equal(t, test.want, actions["admins:update"])
				return c.SendStatus(204)
			})
			response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/actions", nil))
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, http.StatusNoContent, response.StatusCode)
		})
	}
}
