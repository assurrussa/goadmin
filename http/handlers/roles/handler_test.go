package roles_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	"github.com/assurrussa/goadmin/http/handlers/roles"
	rolesmocks "github.com/assurrussa/goadmin/http/handlers/roles/mocks"
	"github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	"github.com/assurrussa/goadmin/infrastructure/core/session"
	tests "github.com/assurrussa/goadmin/infrastructure/fiber/testsupport/utilst"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	identity "github.com/assurrussa/goadmin/internal/identity"
	models2 "github.com/assurrussa/goadmin/models"
)

const firstRoleUUID = "uuid-1"

type TestSuite struct {
	suite.Suite

	listMock           *rolesmocks.MocklistRolesUseCase
	getMock            *rolesmocks.MockgetRoleUseCase
	createMock         *rolesmocks.MockcreateRoleUseCase
	updateMock         *rolesmocks.MockupdateRoleUseCase
	deleteMock         *rolesmocks.MockdeleteRoleUseCase
	setPermMock        *rolesmocks.MocksetPermissionsUseCase
	assignMock         *rolesmocks.MockassignAdminRolesUseCase
	listPermMock       *rolesmocks.MocklistPermissionsUseCase
	listRolePermMock   *rolesmocks.MocklistRolePermissionsUseCase
	listAdminRolesMock *rolesmocks.MocklistAdminRolesUseCase
	listAllRolesMock   *rolesmocks.MocklistAllRolesUseCase

	app     *fiber.App
	testApp *adminappt.App

	handler   *roles.Handler
	adminRepo *fakeAdminRepo
}

type suiteOption func(*suiteConfig)

type suiteConfig struct {
	readGuard func(fiber.Ctx) error
}

func withReadGuard(fn func(fiber.Ctx) error) suiteOption {
	return func(cfg *suiteConfig) {
		cfg.readGuard = fn
	}
}

func NewHandlerSuite(t *testing.T, opts ...suiteOption) (context.Context, context.CancelFunc, *TestSuite) {
	t.Helper()

	cfg := suiteConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	return tests.NewSuite[*TestSuite](t, func(t *testing.T, _ context.Context) *TestSuite {
		t.Helper()

		testApp := adminappt.NewAppTest(t)
		readGuard := func(c fiber.Ctx) error {
			return c.Next()
		}
		if cfg.readGuard != nil {
			readGuard = cfg.readGuard
		}

		testApp.ExpertGuard(integrationroles.PermissionDomainRoles, integrationroles.PermissionActionRead, readGuard).AnyTimes()
		testApp.ExpertGuard(integrationroles.PermissionDomainRoles, integrationroles.PermissionActionCreate).AnyTimes()
		testApp.ExpertGuard(integrationroles.PermissionDomainRoles, integrationroles.PermissionActionUpdate).AnyTimes()
		testApp.ExpertGuard(integrationroles.PermissionDomainRoles, integrationroles.PermissionActionDelete).AnyTimes()
		testApp.ExpertGuard(integrationroles.PermissionDomainRoles, integrationroles.PermissionActionAssign).AnyTimes()
		testApp.MockRoleService.EXPECT().
			AdminGuardCheck(
				gomock.Any(),
				gomock.Any(),
				gomock.AssignableToTypeOf(integrationroles.PermissionKey{}),
			).
			Return(true).
			AnyTimes()

		ctrl := gomock.NewController(t)
		listMock := rolesmocks.NewMocklistRolesUseCase(ctrl)
		getMock := rolesmocks.NewMockgetRoleUseCase(ctrl)
		createMock := rolesmocks.NewMockcreateRoleUseCase(ctrl)
		updateMock := rolesmocks.NewMockupdateRoleUseCase(ctrl)
		deleteMock := rolesmocks.NewMockdeleteRoleUseCase(ctrl)
		setPermMock := rolesmocks.NewMocksetPermissionsUseCase(ctrl)
		assignMock := rolesmocks.NewMockassignAdminRolesUseCase(ctrl)
		listPermMock := rolesmocks.NewMocklistPermissionsUseCase(ctrl)
		listRolePermMock := rolesmocks.NewMocklistRolePermissionsUseCase(ctrl)
		listAdminRolesMock := rolesmocks.NewMocklistAdminRolesUseCase(ctrl)
		listAllRolesMock := rolesmocks.NewMocklistAllRolesUseCase(ctrl)
		adminRepo := newFakeAdminRepo()

		handler := roles.NewHandler(
			testApp.App,
			adminRepo,
			roles.UseCases{
				ListRoles:           listMock,
				GetRole:             getMock,
				CreateRole:          createMock,
				UpdateRole:          updateMock,
				DeleteRole:          deleteMock,
				SetPermissions:      setPermMock,
				AssignAdminRoles:    assignMock,
				ListPermissions:     listPermMock,
				ListRolePermissions: listRolePermMock,
				ListAdminRoles:      listAdminRolesMock,
				ListAllRoles:        listAllRolesMock,
			},
		)

		app := fiber.New(fiber.Config{ErrorHandler: testApp.InertiaManager.MiddlewareErrorListener()})
		app.Use(func(c fiber.Ctx) error {
			c.Locals(session.AuthAdminKey.String(), &models2.SessionAdmin{ID: 1})
			return c.Next()
		})

		handler.RegisterGroupRoutes(app)

		return &TestSuite{
			app:                app,
			testApp:            testApp,
			handler:            handler,
			adminRepo:          adminRepo,
			listMock:           listMock,
			getMock:            getMock,
			createMock:         createMock,
			updateMock:         updateMock,
			deleteMock:         deleteMock,
			setPermMock:        setPermMock,
			assignMock:         assignMock,
			listPermMock:       listPermMock,
			listRolePermMock:   listRolePermMock,
			listAdminRolesMock: listAdminRolesMock,
			listAllRolesMock:   listAllRolesMock,
		}
	})
}

func TestUploads_List_Success(t *testing.T) {
	_, _, ts := NewHandlerSuite(t)

	ts.listMock.EXPECT().
		Handle(gomock.Any(), integrationroles.ListRolesRequest{IncludeSystem: true}).
		Return(integrationroles.ListRolesResponse{
			Roles: []integrationroles.ListRolesRole{
				{ID: 1, UUID: firstRoleUUID, Slug: "cms_editor", Name: "CMS Editor", IsSystem: true},
				{ID: 2, UUID: "uuid-2", Slug: "cms_custom", Name: "Custom", IsSystem: false},
			},
		}, nil).
		Times(1)

	resp, err := ts.app.Test(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/roles/data", nil))
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()
	ts.Equal(200, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	ts.Require().NoError(err)
	require.Contains(t, string(body), `"slug":"cms_editor"`)
	require.Contains(t, string(body), `"scope":"Только CMS"`)
	require.Contains(t, string(body), `"slug":"cms_custom"`)
	require.Contains(t, string(body), `"scope":"По выбранным разрешениям"`)
	require.Contains(t, string(body), `"true":{"label":"Системная","variant":"warning"}`)
}

func TestHandler_AvailablePermissionsUsesLiteralRoute(t *testing.T) {
	_, _, ts := NewHandlerSuite(t)
	ts.listPermMock.EXPECT().
		Handle(gomock.Any(), integrationroles.ListPermissionsRequest{}).
		Return(integrationroles.ListPermissionsResponse{}, nil)

	resp, err := ts.app.Test(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/roles/permissions", nil))
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHandler_CreateRole(t *testing.T) {
	_, _, ts := NewHandlerSuite(t)

	var captured integrationroles.CreateRoleRequest
	ts.createMock.EXPECT().
		Handle(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req integrationroles.CreateRoleRequest) (integrationroles.CreateRoleResponse, error) {
			captured = req
			return integrationroles.CreateRoleResponse{RoleID: 99}, nil
		}).
		Times(1)

	requestBody := `{"slug":" managers ","name":"Managers","permissions":["roles:read"]}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/roles", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "/roles")

	resp, err := ts.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	ts.Equal(fiber.StatusFound, resp.StatusCode)
	ts.Equal("managers", captured.Slug)
	ts.Equal("Managers", captured.Name)
	require.Equal(t, []string{"roles:read"}, captured.PermissionKeys)
}

func TestHandler_ListRoles_PermissionDenied(t *testing.T) {
	denyGuard := func(fiber.Ctx) error {
		return fiber.NewError(fiber.StatusForbidden, "permission denied")
	}

	_, _, ts := NewHandlerSuite(t, withReadGuard(denyGuard))

	resp, err := ts.app.Test(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/roles", nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}

func TestHandler_RoleAdmins_ReturnsAssigned(t *testing.T) {
	_, cancel, ts := NewHandlerSuite(t)
	defer cancel()

	ts.adminRepo.setRoles(1, []string{"Managers"}) //nolint:goconst // required

	ts.getMock.EXPECT().
		Handle(gomock.Any(), integrationroles.GetRoleRequest{RoleID: 10, IncludePermissions: false}).
		Return(integrationroles.GetRoleResponse{ID: 10, Name: "Managers"}, nil).
		Times(1)
	ts.listAllRolesMock.EXPECT().
		Handle(gomock.Any(), integrationroles.ListAllRolesRequest{}).
		Return(integrationroles.ListAllRolesResponse{SubjectRoles: []integrationroles.SubjectRole{
			{SubjectID: "123e4567-e89b-12d3-a456-426614174216", RoleID: 10},
		}}, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/roles/10/admins", nil)
	resp, err := ts.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, fiber.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "Admin One")
}

func TestHandler_AssignRoles_DeniesSuperRoleAssignment(t *testing.T) {
	_, _, ts := NewHandlerSuite(t)

	ts.listAdminRolesMock.EXPECT().
		Handle(gomock.Any(), roles.ListAdminRolesRequest{AdminID: 2}).
		Return(roles.ListAdminRolesResponse{Roles: []integrationroles.Role{}}, nil).
		Times(1)
	ts.getMock.EXPECT().
		Handle(gomock.Any(), integrationroles.GetRoleRequest{RoleID: 5, IncludePermissions: false}).
		Return(integrationroles.GetRoleResponse{ID: 5, Slug: integrationroles.SuperAdminRole}, nil).
		Times(1)
	ts.testApp.MockRoleService.EXPECT().
		IsSuperAdmin(gomock.Any(), int64(1)).
		Return(false).
		Times(1)
	ts.assignMock.EXPECT().
		Handle(gomock.Any(), gomock.Any()).
		Times(0)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/roles/assign", strings.NewReader(`{"adminId":2,"roleIds":[5]}`)) //nolint:lll // required
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, fiber.StatusFound, resp.StatusCode)
}

func TestHandler_AssignRoles_DeniesChangesForSuperAdmin(t *testing.T) {
	_, _, ts := NewHandlerSuite(t)

	ts.listAdminRolesMock.EXPECT().
		Handle(gomock.Any(), roles.ListAdminRolesRequest{AdminID: 2}).
		Return(roles.ListAdminRolesResponse{Roles: []integrationroles.Role{
			{ID: 1, Slug: integrationroles.SuperAdminRole},
		}}, nil).
		Times(1)
	ts.getMock.EXPECT().
		Handle(gomock.Any(), integrationroles.GetRoleRequest{RoleID: 5, IncludePermissions: false}).
		Return(integrationroles.GetRoleResponse{ID: 5, Slug: "manager"}, nil). //nolint:goconst // required
		Times(1)
	ts.testApp.MockRoleService.EXPECT().
		IsSuperAdmin(gomock.Any(), int64(1)).
		Return(false).
		Times(1)
	ts.assignMock.EXPECT().
		Handle(gomock.Any(), gomock.Any()).
		Times(0)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/roles/assign", strings.NewReader(`{"adminId":2,"roleIds":[5]}`)) //nolint:lll // required
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, fiber.StatusFound, resp.StatusCode)
}

func TestHandler_AttachRoleToAdmin(t *testing.T) {
	_, cancel, ts := NewHandlerSuite(t)
	defer cancel()

	ts.adminRepo.setRoles(1, []string{"User"}) //nolint:goconst // required

	ts.getMock.EXPECT().
		Handle(gomock.Any(), integrationroles.GetRoleRequest{RoleID: 5, IncludePermissions: false}).
		Return(integrationroles.GetRoleResponse{ID: 5, Slug: "manager"}, nil).
		Times(1)

	ts.listAdminRolesMock.EXPECT().
		Handle(gomock.Any(), roles.ListAdminRolesRequest{AdminID: 1}).
		Return(roles.ListAdminRolesResponse{Roles: []integrationroles.Role{{ID: 2}}}, nil).
		Times(1)

	ts.assignMock.EXPECT().
		Handle(gomock.Any(), roles.AssignAdminRolesRequest{AdminID: 1, RoleIDs: []int64{2, 5}}).
		Return(roles.AssignAdminRolesResponse{Assigned: 2}, nil).
		Times(1)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/roles/5/admins/attach", strings.NewReader(`{"adminId":1}`)) //nolint:lll // required
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, fiber.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), `"id":1`)
}

func TestHandler_AttachRoleToAdmin_InertiaRedirect(t *testing.T) {
	_, cancel, ts := NewHandlerSuite(t)
	defer cancel()

	ts.adminRepo.setRoles(1, []string{"User"})

	ts.getMock.EXPECT().
		Handle(gomock.Any(), integrationroles.GetRoleRequest{RoleID: 5, IncludePermissions: false}).
		Return(integrationroles.GetRoleResponse{ID: 5, Slug: "manager"}, nil).
		Times(1)

	ts.listAdminRolesMock.EXPECT().
		Handle(gomock.Any(), roles.ListAdminRolesRequest{AdminID: 1}).
		Return(roles.ListAdminRolesResponse{Roles: []integrationroles.Role{{ID: 2}}}, nil).
		Times(1)

	ts.assignMock.EXPECT().
		Handle(gomock.Any(), roles.AssignAdminRolesRequest{AdminID: 1, RoleIDs: []int64{2, 5}}).
		Return(roles.AssignAdminRolesResponse{Assigned: 2}, nil).
		Times(1)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/roles/5/admins/attach", strings.NewReader(`{"adminId":1}`)) //nolint:lll // required
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(goinertia.HeaderInertia, "true")
	req.Header.Set(goinertia.HeaderVersion, "test")
	req.Header.Set(fiber.HeaderReferer, "/roles/5")

	resp, err := ts.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, fiber.StatusFound, resp.StatusCode)
	require.Equal(t, "/roles/5", resp.Header.Get("location"))
}

func TestHandler_AttachRoleToAdmin_DeniesSuperRole(t *testing.T) {
	_, cancel, ts := NewHandlerSuite(t)
	defer cancel()

	ts.getMock.EXPECT().
		Handle(gomock.Any(), integrationroles.GetRoleRequest{RoleID: 5, IncludePermissions: false}).
		Return(integrationroles.GetRoleResponse{ID: 5, Slug: integrationroles.SuperAdminRole}, nil).
		Times(1)
	ts.listAdminRolesMock.EXPECT().
		Handle(gomock.Any(), roles.ListAdminRolesRequest{AdminID: 1}).
		Return(roles.ListAdminRolesResponse{Roles: []integrationroles.Role{{ID: 2}}}, nil).
		Times(1)
	ts.testApp.MockRoleService.EXPECT().
		IsSuperAdmin(gomock.Any(), int64(1)).
		Return(false).
		Times(1)
	ts.assignMock.EXPECT().
		Handle(gomock.Any(), gomock.Any()).
		Times(0)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/roles/5/admins/attach", strings.NewReader(`{"adminId":1}`)) //nolint:lll // required
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, fiber.StatusFound, resp.StatusCode)
}

func TestHandler_AttachRoleToAdmin_DeniesSuperAdminTarget(t *testing.T) {
	_, cancel, ts := NewHandlerSuite(t)
	defer cancel()

	ts.getMock.EXPECT().
		Handle(gomock.Any(), integrationroles.GetRoleRequest{RoleID: 5, IncludePermissions: false}).
		Return(integrationroles.GetRoleResponse{ID: 5, Slug: "manager"}, nil).
		Times(1)
	ts.listAdminRolesMock.EXPECT().
		Handle(gomock.Any(), roles.ListAdminRolesRequest{AdminID: 1}).
		Return(roles.ListAdminRolesResponse{Roles: []integrationroles.Role{
			{ID: 2, Slug: integrationroles.SuperAdminRole},
		}}, nil).
		Times(1)
	ts.testApp.MockRoleService.EXPECT().
		IsSuperAdmin(gomock.Any(), int64(1)).
		Return(false).
		Times(1)
	ts.assignMock.EXPECT().
		Handle(gomock.Any(), gomock.Any()).
		Times(0)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/roles/5/admins/attach", strings.NewReader(`{"adminId":1}`)) //nolint:lll // required
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, fiber.StatusFound, resp.StatusCode)
}

func TestHandler_DetachRoleFromAdmin(t *testing.T) {
	_, cancel, ts := NewHandlerSuite(t)
	defer cancel()

	ts.adminRepo.setRoles(1, []string{"Manager"})

	ts.getMock.EXPECT().
		Handle(gomock.Any(), integrationroles.GetRoleRequest{RoleID: 5, IncludePermissions: false}).
		Return(integrationroles.GetRoleResponse{ID: 5, Slug: "manager"}, nil).
		Times(1)

	ts.listAdminRolesMock.EXPECT().
		Handle(gomock.Any(), roles.ListAdminRolesRequest{AdminID: 1}).
		Return(roles.ListAdminRolesResponse{Roles: []integrationroles.Role{{ID: 5}, {ID: 7}}}, nil).
		Times(1)

	ts.assignMock.EXPECT().
		Handle(gomock.Any(), roles.AssignAdminRolesRequest{AdminID: 1, RoleIDs: []int64{7}}).
		Return(roles.AssignAdminRolesResponse{Assigned: 1}, nil).
		Times(1)

	ts.testApp.MockRoleService.EXPECT().IsAdoptedDetachRole(gomock.Any(), int64(1), int64(1), int64(5)).
		Return(true, nil).Times(1)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/roles/5/admins/detach", strings.NewReader(`{"adminId":1}`)) //nolint:lll // required
	req.Header.Set("Content-Type", "application/json")
	resp, err := ts.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, fiber.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), `"adminId":1`)
}

func TestHandler_DetachRoleFromAdmin_InertiaRedirect(t *testing.T) {
	_, cancel, ts := NewHandlerSuite(t)
	defer cancel()

	ts.adminRepo.setRoles(1, []string{"Manager"})

	ts.getMock.EXPECT().
		Handle(gomock.Any(), integrationroles.GetRoleRequest{RoleID: 5, IncludePermissions: false}).
		Return(integrationroles.GetRoleResponse{ID: 5, Slug: "manager"}, nil).
		Times(1)

	ts.listAdminRolesMock.EXPECT().
		Handle(gomock.Any(), roles.ListAdminRolesRequest{AdminID: 1}).
		Return(roles.ListAdminRolesResponse{Roles: []integrationroles.Role{{ID: 5}, {ID: 7}}}, nil).
		Times(1)

	ts.assignMock.EXPECT().
		Handle(gomock.Any(), roles.AssignAdminRolesRequest{AdminID: 1, RoleIDs: []int64{7}}).
		Return(roles.AssignAdminRolesResponse{Assigned: 1}, nil).
		Times(1)

	ts.testApp.MockRoleService.EXPECT().IsAdoptedDetachRole(gomock.Any(), int64(1), int64(1), int64(5)).
		Return(true, nil).Times(1)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/roles/5/admins/detach", strings.NewReader(`{"adminId":1}`)) //nolint:lll // required
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(goinertia.HeaderInertia, "true")
	req.Header.Set(goinertia.HeaderVersion, "test")
	req.Header.Set(fiber.HeaderReferer, "/roles/5")

	resp, err := ts.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, fiber.StatusFound, resp.StatusCode)
	require.Equal(t, "/roles/5", resp.Header.Get("location"))
}

func TestHandler_DetachRoleFromAdmin_DeniesSuperAdminTarget(t *testing.T) {
	_, cancel, ts := NewHandlerSuite(t)
	defer cancel()

	ts.getMock.EXPECT().
		Handle(gomock.Any(), integrationroles.GetRoleRequest{RoleID: 5, IncludePermissions: false}).
		Return(integrationroles.GetRoleResponse{ID: 5, Slug: "manager"}, nil).
		Times(1)
	ts.listAdminRolesMock.EXPECT().
		Handle(gomock.Any(), roles.ListAdminRolesRequest{AdminID: 1}).
		Return(roles.ListAdminRolesResponse{Roles: []integrationroles.Role{
			{ID: 5, Slug: "manager"},
			{ID: 2, Slug: integrationroles.SuperAdminRole},
		}}, nil).
		Times(1)
	ts.testApp.MockRoleService.EXPECT().
		IsSuperAdmin(gomock.Any(), int64(1)).
		Return(false).
		Times(1)
	ts.assignMock.EXPECT().
		Handle(gomock.Any(), gomock.Any()).
		Times(0)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/roles/5/admins/detach", strings.NewReader(`{"adminId":1}`)) //nolint:lll // required
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, fiber.StatusFound, resp.StatusCode)
}

func TestHandler_EditFormSuperAdminBlocked(t *testing.T) {
	_, _, ts := NewHandlerSuite(t)

	ts.getMock.EXPECT().
		Handle(gomock.Any(), integrationroles.GetRoleRequest{RoleID: 1, IncludePermissions: true}).
		Return(integrationroles.GetRoleResponse{
			ID:       1,
			Slug:     integrationroles.SuperAdminRole,
			Name:     "Super Admin", //nolint:goconst // required
			UUID:     firstRoleUUID,
			IsSystem: true,
		}, nil).
		Times(1)
	ts.listPermMock.EXPECT().
		Handle(gomock.Any(), integrationroles.ListPermissionsRequest{}).
		Return(integrationroles.ListPermissionsResponse{}, nil).
		Times(1)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/roles/1/edit", nil)
	req.Header.Set("Referer", "/roles")

	resp, err := ts.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestHandler_UpdateSuperAdminBlocked(t *testing.T) {
	_, _, ts := NewHandlerSuite(t)

	ts.getMock.EXPECT().
		Handle(gomock.Any(), integrationroles.GetRoleRequest{RoleID: 1, IncludePermissions: false}).
		Return(integrationroles.GetRoleResponse{
			ID:   1,
			Slug: integrationroles.SuperAdminRole,
			Name: "Super Admin",
			UUID: firstRoleUUID,
		}, nil).
		Times(1)
	ts.testApp.MockRoleService.EXPECT().
		IsSuperAdmin(gomock.Any(), int64(1)).
		Return(false).
		Times(1)
	ts.updateMock.EXPECT().
		Handle(gomock.Any(), gomock.Any()).
		Times(0)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPut, "/roles/1", strings.NewReader(`{"name":"New"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "/roles/1/edit")

	resp, err := ts.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, fiber.StatusFound, resp.StatusCode)
}

func TestHandler_UpdateSuperAdminKeepsSlug(t *testing.T) {
	_, _, ts := NewHandlerSuite(t)

	ts.getMock.EXPECT().
		Handle(gomock.Any(), integrationroles.GetRoleRequest{RoleID: 1, IncludePermissions: false}).
		Return(integrationroles.GetRoleResponse{
			ID:   1,
			Slug: integrationroles.SuperAdminRole,
			Name: "Super Admin",
			UUID: firstRoleUUID,
		}, nil).
		Times(1)
	ts.testApp.MockRoleService.EXPECT().
		IsSuperAdmin(gomock.Any(), int64(1)).
		Return(true).
		Times(1)
	ts.updateMock.EXPECT().
		Handle(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req integrationroles.UpdateRoleRequest) (integrationroles.UpdateRoleResponse, error) {
			require.Equal(t, integrationroles.SuperAdminRole, req.Slug)
			return integrationroles.UpdateRoleResponse{}, nil
		}).
		Times(1)

	req := httptest.NewRequestWithContext(context.Background(),
		http.MethodPut,
		"/roles/1",
		strings.NewReader(`{"slug":"new-admin","name":"Updated"}`))

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "/roles/1/edit")

	resp, err := ts.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, fiber.StatusFound, resp.StatusCode)
}

func TestHandler_DeleteSuperAdminBlocked(t *testing.T) {
	_, _, ts := NewHandlerSuite(t)

	ts.getMock.EXPECT().
		Handle(gomock.Any(), integrationroles.GetRoleRequest{RoleID: 1, IncludePermissions: false}).
		Return(integrationroles.GetRoleResponse{
			ID:   1,
			Slug: integrationroles.SuperAdminRole,
			Name: "Super Admin",
			UUID: firstRoleUUID,
		}, nil).
		Times(1)
	ts.testApp.MockRoleService.EXPECT().
		IsSuperAdmin(gomock.Any(), int64(1)).
		Return(false).
		Times(1)
	ts.deleteMock.EXPECT().
		Handle(gomock.Any(), integrationroles.DeleteRoleRequest{RoleID: 1}).
		Times(0)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/roles/1", nil)
	req.Header.Set("Referer", "/roles")

	resp, err := ts.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, fiber.StatusFound, resp.StatusCode)
}

func TestHandler_UpdatePermissionsSuperAdminBlocked(t *testing.T) {
	_, _, ts := NewHandlerSuite(t)

	ts.getMock.EXPECT().
		Handle(gomock.Any(), integrationroles.GetRoleRequest{RoleID: 1, IncludePermissions: false}).
		Return(integrationroles.GetRoleResponse{
			ID:   1,
			Slug: integrationroles.SuperAdminRole,
			Name: "Super Admin",
			UUID: firstRoleUUID,
		}, nil).
		Times(1)
	ts.testApp.MockRoleService.EXPECT().
		IsSuperAdmin(gomock.Any(), int64(1)).
		Return(false).
		Times(1)
	ts.setPermMock.EXPECT().
		Handle(gomock.Any(), gomock.Any()).
		Times(0)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/roles/1/permissions", strings.NewReader(`{"permissions":["roles:read"]}`)) //nolint:lll // required
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "/roles/1")

	resp, err := ts.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, fiber.StatusFound, resp.StatusCode)
}

type fakeAdminRepo struct {
	admins map[int64]models2.Admin
}

func newFakeAdminRepo() *fakeAdminRepo {
	return &fakeAdminRepo{
		admins: map[int64]models2.Admin{
			1: {
				ID:        1,
				SubjectID: integrationroles.MustParseSubjectIDString("123e4567-e89b-12d3-a456-426614174216"),
				UUID:      identity.UserID(uuid.MustParse("123e4567-e89b-12d3-a456-426614174116")),
				Name:      "Admin One",
				LastName:  "Tester",
				Email:     "admin1@example.com",
				Username:  "admin1",
				Data: &models2.AdminData{
					Roles: []string{"Managers"},
				},
			},
			2: {
				ID:        2,
				SubjectID: integrationroles.MustParseSubjectIDString("123e4567-e89b-12d3-a456-426614174217"),
				UUID:      identity.UserID(uuid.MustParse("123e4567-e89b-12d3-a456-426614174117")),
				Name:      "Admin Two",
				LastName:  "Tester",
				Email:     "admin2@example.com",
				Username:  "admin2",
				Data: &models2.AdminData{
					Roles: []string{"User"},
				},
			},
		},
	}
}

func (f *fakeAdminRepo) GetByID(_ context.Context, id int64) (models2.Admin, error) {
	adm, ok := f.admins[id]
	if !ok {
		return models2.Admin{}, fmt.Errorf("admin %d not found", id)
	}
	return adm, nil
}

func (f *fakeAdminRepo) GetBySubjectID(_ context.Context, subjectID integrationroles.SubjectID) (models2.Admin, error) {
	for _, adm := range f.admins {
		if adm.AuthSubjectID() == subjectID {
			return adm, nil
		}
	}

	return models2.Admin{}, fmt.Errorf("admin subject %s not found", subjectID.String())
}

func (f *fakeAdminRepo) GetByUUID(_ context.Context, id identity.UserID) (models2.Admin, error) {
	for _, adm := range f.admins {
		if adm.UUID == id {
			return adm, nil
		}
	}

	return models2.Admin{}, fmt.Errorf("admin %s not found", id.String())
}

func (f *fakeAdminRepo) GetList(_ context.Context, _ datagrid.Filtered) ([]models2.Admin, int, error) {
	list := make([]models2.Admin, 0, len(f.admins))
	for _, adm := range f.admins {
		list = append(list, adm)
	}
	sort.SliceStable(list, func(i, j int) bool {
		return list[i].ID < list[j].ID
	})
	return list, len(list), nil
}

//nolint:unparam // it's valid
func (f *fakeAdminRepo) setRoles(id int64, roles []string) {
	adm, ok := f.admins[id]
	if !ok {
		return
	}
	if adm.Data == nil {
		adm.Data = &models2.AdminData{}
	}
	adm.Data.Roles = append([]string(nil), roles...)
	f.admins[id] = adm
}
