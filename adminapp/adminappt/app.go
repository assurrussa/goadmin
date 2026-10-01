package adminappt

import (
	"testing"

	logger "github.com/assurrussa/gologger"
	inmemeventstream "github.com/assurrussa/gowebsocket/eventstream/inmem"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
	"go.uber.org/mock/gomock"

	"github.com/assurrussa/goadmin/adminapp"
	adminappmocks "github.com/assurrussa/goadmin/adminapp/mocks"
	utilst "github.com/assurrussa/goadmin/infrastructure/fiber/testsupport/utilst"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
	inertiat "github.com/assurrussa/goadmin/infrastructure/inertia/testsupport"
	outboxtest "github.com/assurrussa/goadmin/infrastructure/outbox/testsupport"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/envs"
	"github.com/assurrussa/goadmin/services/adminservice"
)

type App struct {
	Ctrl *gomock.Controller

	MockTransaction     *outboxtest.MockTxManager
	MockOutbox          *adminappmocks.MockOutboxPutter
	MockPermissionGuard *adminappmocks.MockPermissionGuard
	MockRoleService     *adminappmocks.MockRolesService

	EventStreamService *inmemeventstream.Service
	InertiaManager     *goinertia.Inertia
	AdminService       *adminservice.Service
	SessionStore       *session.Store

	App *adminapp.App
}

func NewAppTest(t *testing.T) *App {
	t.Helper()

	ctrl := gomock.NewController(t)
	store := utilst.NewTestStore()
	mockRoleService := adminappmocks.NewMockRolesService(ctrl)
	adminService := adminservice.NewService(store, mockRoleService)
	mockTransaction := outboxtest.NewMockTxManager(ctrl)
	inertiaManager := inertiat.NewForTest("")
	eventStreamService := inmemeventstream.New()
	outboxMock := adminappmocks.NewMockOutboxPutter(ctrl)
	permissionGuardMock := adminappmocks.NewMockPermissionGuard(ctrl)
	log := logger.Discard()
	mainApp := adminapp.Must(adminapp.NewOptions(
		inertiaManager, permissionGuardMock, mockRoleService, adminService, store, eventStreamService, outboxMock,
		mockTransaction, log, envs.EnvProd, "https://ceph.localhost",
	))

	mainApp.SetCommandTransaction(mockTransaction.RunInTx)
	return &App{
		Ctrl:                ctrl,
		MockTransaction:     mockTransaction,
		MockOutbox:          outboxMock,
		MockRoleService:     mockRoleService,
		MockPermissionGuard: permissionGuardMock,
		EventStreamService:  eventStreamService,
		InertiaManager:      inertiaManager,
		AdminService:        adminService,
		SessionStore:        store,
		App:                 mainApp,
	}
}

func (a *App) ExpertGuard(
	domain integrationroles.PermissionDomain,
	action integrationroles.PermissionAction,
	fns ...func(c fiber.Ctx) error,
) *gomock.Call {
	fn := func(c fiber.Ctx) error {
		return c.Next()
	}
	for _, fnNew := range fns {
		fn = fnNew
		break
	}

	return a.MockPermissionGuard.EXPECT().
		AdminGuard(integrationroles.NewPermissionKey(domain, action)).
		Return(fn)
}
