package permissions_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	"github.com/assurrussa/goadmin/http/handlers/permissions"
	permissionsmocks "github.com/assurrussa/goadmin/http/handlers/permissions/mocks"
	"github.com/assurrussa/goadmin/infrastructure/core/session"
	tests "github.com/assurrussa/goadmin/infrastructure/fiber/testsupport/utilst"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/models"
)

type TestSuite struct {
	suite.Suite

	listMock    *permissionsmocks.MocklistPermissionsUseCase
	serviceMock *permissionsmocks.MockpermissionService

	app     *fiber.App
	handler *permissions.Handler
}

type suiteOption func(*suiteConfig)

type suiteConfig struct {
	readGuard             func(fiber.Ctx) error
	permissionDefinitions []integrationroles.PermissionDefinition
}

func withReadGuard(fn func(fiber.Ctx) error) suiteOption {
	return func(cfg *suiteConfig) {
		cfg.readGuard = fn
	}
}

func withPermissionDefinitions(definitions ...integrationroles.PermissionDefinition) suiteOption {
	return func(cfg *suiteConfig) {
		cfg.permissionDefinitions = append(cfg.permissionDefinitions, definitions...)
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

		testApp.ExpertGuard(integrationroles.PermissionDomainPermissions, integrationroles.PermissionActionRead, readGuard).AnyTimes()
		testApp.ExpertGuard(integrationroles.PermissionDomainPermissions, integrationroles.PermissionActionSync).AnyTimes()
		testApp.App.SetPermissionDefinitions(cfg.permissionDefinitions)

		ctrl := gomock.NewController(t)
		listMock := permissionsmocks.NewMocklistPermissionsUseCase(ctrl)
		serviceMock := permissionsmocks.NewMockpermissionService(ctrl)

		handler := permissions.NewHandler(testApp.App, listMock, serviceMock)

		app := fiber.New()
		app.Use(func(c fiber.Ctx) error {
			c.Locals(session.AuthAdminKey.String(), &models.SessionAdmin{ID: 1})
			return c.Next()
		})

		handler.RegisterGroupRoutes(app)

		return &TestSuite{
			app:         app,
			handler:     handler,
			listMock:    listMock,
			serviceMock: serviceMock,
		}
	})
}

func TestPermissions_List(t *testing.T) {
	_, _, ts := NewHandlerSuite(t)

	ts.listMock.EXPECT().
		Handle(gomock.Any(), integrationroles.ListPermissionsRequest{}).
		Return(integrationroles.ListPermissionsResponse{
			Permissions: []integrationroles.ListPermissionsPermission{
				{ID: 1, UUID: "uuid-1", Domain: "roles", Action: "read"},
			},
		}, nil).
		Times(1)

	resp, err := ts.app.Test(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/permissions/data", nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestPermissions_Sync(t *testing.T) {
	_, _, ts := NewHandlerSuite(t)

	ts.serviceMock.EXPECT().
		EnsurePermissions(gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)
	ts.serviceMock.EXPECT().
		CreatePermissions(gomock.Any(), gomock.Any()).
		Return([]integrationroles.Permission{}, nil).
		Times(1)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/permissions/sync", strings.NewReader(""))
	req.Header.Set("Referer", "/permissions")

	resp, err := ts.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, fiber.StatusFound, resp.StatusCode)
}

func TestPermissions_SyncIncludesHostDefinitions(t *testing.T) {
	hostDefinition := integrationroles.PermissionDefinition{
		Key:         integrationroles.NewPermissionKey("exercises", integrationroles.PermissionActionRead),
		Description: "Read exercises",
	}

	_, _, ts := NewHandlerSuite(t, withPermissionDefinitions(hostDefinition))

	ts.serviceMock.EXPECT().
		EnsurePermissions(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, inputs []integrationroles.CreatePermissionInput) error {
			for _, input := range inputs {
				if input.Key == hostDefinition.Key && input.Description == hostDefinition.Description {
					return nil
				}
			}
			t.Fatalf("expected host definition in sync input: %+v", inputs)
			return nil
		}).
		Times(1)
	ts.serviceMock.EXPECT().
		CreatePermissions(gomock.Any(), gomock.Any()).
		Return([]integrationroles.Permission{}, nil).
		Times(1)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/permissions/sync", strings.NewReader(""))
	req.Header.Set("Referer", "/permissions")

	resp, err := ts.app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, fiber.StatusFound, resp.StatusCode)
}

func TestPermissions_GuardDenied(t *testing.T) {
	denyGuard := func(fiber.Ctx) error {
		return fiber.NewError(fiber.StatusForbidden, "permission denied")
	}

	_, _, ts := NewHandlerSuite(t, withReadGuard(denyGuard))

	resp, err := ts.app.Test(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/permissions", nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, fiber.StatusForbidden, resp.StatusCode)
}
