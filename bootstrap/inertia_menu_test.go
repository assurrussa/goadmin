package bootstrap //nolint:testpackage // verifies shared sidebar and breadcrumb loaders

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/adminbreadcrumbs"
	"github.com/assurrussa/goadmin/infrastructure/core/menu"
	coresession "github.com/assurrussa/goadmin/infrastructure/core/session"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/models"
)

type menuKeyChecker struct {
	action integrationroles.PermissionAction
}

func (menuKeyChecker) IsSuperAdmin(context.Context, int64) bool { return false }
func (checker menuKeyChecker) AdminCan(
	_ context.Context, _ int64, domain integrationroles.PermissionDomain, action integrationroles.PermissionAction,
) bool {
	return domain == "users" && action == checker.action
}

func TestSharedMenuAndBreadcrumbsUseAnyPermissionKeys(t *testing.T) {
	const studentsPath = "/students"
	for _, action := range []integrationroles.PermissionAction{"", "read", "create", "update", "delete"} {
		t.Run(string(action), func(t *testing.T) {
			extra := menu.Menu{Sections: []menu.Section{{Key: "students", Items: []menu.Item{{
				Name: "Students", Href: studentsPath, AnyPermissionKeys: []integrationroles.PermissionKey{
					integrationroles.NewPermissionKey("users", "read"), integrationroles.NewPermissionKey("users", "create"),
					integrationroles.NewPermissionKey("users", "update"), integrationroles.NewPermissionKey("users", "delete"),
				},
			}}}}}
			checker := menuKeyChecker{action: action}
			app := fiber.New()
			app.Get("/students/42", func(c fiber.Ctx) error {
				c.Locals(coresession.AuthAdminKey.String(), &models.SessionAdmin{ID: 42})
				sidebar, err := loadAdminMenu(checker, extra)(c)
				require.NoError(t, err)
				crumbs, err := loadAdminBreadcrumbs(checker, extra)(c)
				require.NoError(t, err)
				if action == "" {
					visible, ok := sidebar.(menu.Menu)
					require.True(t, ok)
					require.Empty(t, visible.Sections)
					require.Empty(t, crumbs)
				} else {
					visible, ok := sidebar.(menu.Menu)
					require.True(t, ok)
					require.Equal(t, studentsPath, visible.Sections[0].Items[0].Href)
					require.Equal(t, []adminbreadcrumbs.Breadcrumb{{Name: "Students", Href: studentsPath}}, crumbs)
				}
				return c.SendStatus(http.StatusOK)
			})
			resp, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/students/42", nil))
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}
