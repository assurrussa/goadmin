//go:build integration

package host //nolint:testpackage // verifies canonical versus borrowed checker assembly

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/rbac"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/adminapp"
	roleshandler "github.com/assurrussa/goadmin/http/handlers/roles"
	adminsession "github.com/assurrussa/goadmin/infrastructure/core/session"
	inertiat "github.com/assurrussa/goadmin/infrastructure/inertia/testsupport"
	internalauth "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
)

func TestAssemblyRoleMutationsWithSuppliedPermissionChecker(t *testing.T) {
	cfg, dependencies := assemblyFixture(t)
	cfg.Auth.NotificationDelivery = goauth.NotificationDeliveryDisabled
	tx := NewTxManager(dependencies.Database)
	adapter, err := NewAuthAdapter(AuthAdapterConfig{Database: dependencies.Database, TxManager: tx, Runtime: cfg.Auth})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adapter.Close()) })
	checker, err := NewSubjectPermissionChecker(t.Context(), dependencies.Database, tx, DiscardLogger())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, checker.db.Close()) })
	originalGuard := checker.guard
	require.NotSame(t, checker.db, adapter.Runtime().Database())
	input := assemblyInput{
		Config: BootstrapConfig{AdminConfig: cfg.Admin, Capabilities: map[string]bool{"access": true}}, Auth: adapter,
		Services: Services{DB: dependencies.Database, TxManager: tx, SessionStore: session.NewStore(), SubjectPermissions: checker},
	}
	deps, err := buildDependencies(input)
	require.NoError(t, err)
	require.Same(t, originalGuard, checker.guard)
	account, err := adapter.Runtime().ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
		Email: "supplied-checker@example.test", Password: assemblyPassword,
	})
	require.NoError(t, err)
	admin, err := adapter.admins.ProvisionAccount(t.Context(), account, identity.NewUserID())
	require.NoError(t, err)
	roles, err := adapter.Runtime().RBAC(nil)
	require.NoError(t, err)
	super, err := roles.UpsertRole(t.Context(), rbac.Role{Slug: "super_admin", Name: "Super Admin", System: true})
	require.NoError(t, err)
	regular, err := roles.UpsertRole(t.Context(), rbac.Role{Slug: "supplied_checker", Name: "Supplied checker"})
	require.NoError(t, err)
	require.NoError(t, roles.ReplaceSubjectRoles(t.Context(), account.Subject.ID, []int64{super.ID, regular.ID}))
	require.NoError(t, deps.Repos.RolesManager.InSubjectRoleTransaction(t.Context(), []string{account.Subject.ID.String()},
		func(ctx context.Context) error {
			require.True(t, deps.Repos.RolesService.IsSuperAdmin(ctx, admin.ID))
			allowed, err := deps.Repos.RolesService.IsAdoptedDetachRole(ctx, admin.ID, admin.ID, regular.ID)
			require.NoError(t, err)
			require.True(t, allowed)
			allowed, err = deps.Repos.RolesService.IsAdoptedDetachRole(ctx, admin.ID, admin.ID, super.ID)
			require.Error(t, err)
			require.False(t, allowed)
			return nil
		}))
	require.Same(t, originalGuard, checker.guard)
	require.NoError(t, checker.CheckPermission(t.Context(), account.Subject.ID.String(), NewPermissionKey("roles", "assign")))

	// The supported checker constructor has no custom-policy injection. Still
	// exercise a stricter private test double: canonical state checks must never
	// replace the supplied action guard or turn its denial into a grant.
	deniedState := &deniedAssemblyRoles{}
	deniedGuard, err := internalauth.NewGuardServiceWithOptions(internalauth.NewGuardServiceOptions(
		deniedState, deniedState, deniedState, DiscardLogger(),
	))
	require.NoError(t, err)
	checker.guard, checker.rolesGuard = deniedGuard, deniedGuard
	deniedDeps, err := buildDependencies(input)
	require.NoError(t, err)
	require.Same(t, deniedGuard, checker.guard)
	require.True(t, deniedDeps.Repos.RolesService.IsSuperAdmin(t.Context(), admin.ID))
	app := adminapp.Must(adminapp.NewOptions(
		inertiat.NewForTest(""), deniedDeps.System.PermissionGuard, deniedDeps.Repos.RolesService,
		deniedDeps.System.AdminAuthService, input.Services.SessionStore, nil, nil, tx,
		DiscardLogger(), "stage", "https://files.example.test",
	))
	useCases, err := roleshandler.BuildUseCases(deniedDeps.Repos.RolesManager, deniedDeps.Repos.RolesRepo, adapter.admins)
	require.NoError(t, err)
	httpApp := fiber.New()
	httpApp.Use(func(c fiber.Ctx) error {
		c.Locals(adminsession.AuthAdminKey.String(), &models.SessionAdmin{ID: admin.ID})
		return c.Next()
	})
	roleshandler.NewHandler(app, adapter.admins, useCases).RegisterGroupRoutes(httpApp)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		fmt.Sprintf("/roles/%d/admins/detach", regular.ID), strings.NewReader(fmt.Sprintf(`{"adminId":%d}`, admin.ID)))
	request.Header.Set("Content-Type", "application/json")
	response, err := httpApp.Test(request)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusForbidden, response.StatusCode)
	require.True(t, deniedState.assignChecked, "denial must come from roles:assign after roles:read passed")
	remaining, err := roles.SubjectRoles(t.Context(), account.Subject.ID)
	require.NoError(t, err)
	require.Len(t, remaining, 2, "supplied action denial must preserve both assignments")
}

type deniedAssemblyRoles struct{ assignChecked bool }

func (deniedAssemblyRoles) Handle(
	context.Context, internalauth.ListSubjectRolesRequest,
) (internalauth.ListSubjectRolesResponse, error) {
	return internalauth.ListSubjectRolesResponse{}, nil
}

func (d *deniedAssemblyRoles) HasPermission(_ context.Context, _ string, key internalauth.PermissionKey) (bool, error) {
	if key == internalauth.NewPermissionKey("roles", "assign") {
		d.assignChecked = true
	}
	return key == internalauth.NewPermissionKey("roles", "read"), nil
}

func (deniedAssemblyRoles) ListAllPermissionsByRoles(context.Context, []int64) ([]internalauth.PermissionWithRole, error) {
	return nil, nil
}
func (deniedAssemblyRoles) Enabled(context.Context) bool            { return false }
func (deniedAssemblyRoles) SubjectRoles(string) []internalauth.Role { return nil }
func (deniedAssemblyRoles) AllRolePermissions([]internalauth.Role) map[int64]map[int64]internalauth.Permission {
	return nil
}
