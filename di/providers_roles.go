package di

import (
	logger "github.com/assurrussa/gologger"

	"github.com/assurrussa/goadmin/adminapp"
	"github.com/assurrussa/goadmin/bootstrap"
	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	adminrepo "github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminrepo"
	adminrolesfacade "github.com/assurrussa/goadmin/infrastructure/roles/adminroles"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
)

func provideAuthAdapter(
	cfg bootstrap.Config,
	db outbox.StoragePgsqlClient,
	admins *adminrepo.Repo,
) (*integrationroles.Adapter, error) {
	return integrationroles.New(integrationroles.Config{
		Database: db, Runtime: cfg.AuthRuntime, Memberships: admins,
		AutoMigrate: cfg.AuthRuntimeAutoMigrate,
	})
}

func provideRolesService(adapter *integrationroles.Adapter) *integrationroles.Service {
	return adapter.Roles()
}

func provideRolesGuardService(
	listSubject *integrationroles.ListSubjectRolesUseCase,
	roles *integrationroles.Service,
) (*integrationroles.GuardService, error) {
	return integrationroles.NewGuardService(listSubject, roles, logger.Discard())
}

func provideAdminRolesFacade( //nolint:unused // required
	service *integrationroles.GuardService,
	adminRepo adminrolesfacade.AdminRepository,
) (*adminrolesfacade.Service, error) {
	return adminrolesfacade.New(service, adminRepo)
}

func provideRolesGuardPermissionGuard(service *adminrolesfacade.Service) adminapp.PermissionGuard {
	return service
}

func provideRolesGuardRolesService(service *adminrolesfacade.Service) adminapp.RolesService {
	return service
}

func provideUseCaseListRoles(service *integrationroles.Service) *integrationroles.ListRolesUseCase {
	return integrationroles.MustListRolesUseCase(service)
}

func provideUseCaseGetRole(service *integrationroles.Service) *integrationroles.GetRoleUseCase {
	return integrationroles.MustGetRoleUseCase(service)
}

func provideUseCaseCreateRole(service *integrationroles.Service) *integrationroles.CreateRoleUseCase {
	return integrationroles.MustCreateRoleUseCase(service)
}

func provideUseCaseUpdateRole(service *integrationroles.Service) *integrationroles.UpdateRoleUseCase {
	return integrationroles.MustUpdateRoleUseCase(service)
}

func provideUseCaseDeleteRole(service *integrationroles.Service) *integrationroles.DeleteRoleUseCase {
	return integrationroles.MustDeleteRoleUseCase(service)
}

func provideUseCaseSetRolePermissions(service *integrationroles.Service) *integrationroles.SetRolePermissionsUseCase {
	return integrationroles.MustSetRolePermissionsUseCase(service)
}

func provideUseCaseAssignSubjectRoles(service *integrationroles.Service) *integrationroles.AssignSubjectRolesUseCase {
	return integrationroles.MustAssignSubjectRolesUseCase(service)
}

func provideUseCaseListPermissions(service *integrationroles.Service) *integrationroles.ListPermissionsUseCase {
	return integrationroles.MustListPermissionsUseCase(service)
}

func provideUseCaseListRolePermissions(service *integrationroles.Service) *integrationroles.ListRolePermissionsUseCase {
	return integrationroles.MustListRolePermissionsUseCase(service)
}

func provideUseCaseListSubjectRoles(service *integrationroles.Service) *integrationroles.ListSubjectRolesUseCase {
	return integrationroles.MustListSubjectRolesUseCase(service)
}

func provideUseCaseListAllRoles(
	service *integrationroles.Service,
) *integrationroles.ListAllRolesUseCase {
	return integrationroles.MustListAllRolesUseCase(service, service)
}
