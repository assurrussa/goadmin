package bootstrap //nolint:testpackage // Exercises request middleware and server-owned session state.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/assurrussa/goadmin/adminapp"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/models"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/require"
)

type previewOwner struct {
	adminapp.RolesService
	owner bool
}

func (s *previewOwner) IsSuperAdmin(context.Context, int64) bool { return s.owner }

type previewRoles struct{ RolesService }

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
