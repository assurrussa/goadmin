package di

import (
	sharedgodi "github.com/assurrussa/godi"

	"github.com/assurrussa/goadmin/bootstrap"
	"github.com/assurrussa/goadmin/infrastructure/outbox"
)

// MergeDependencies combines dependency groups into a single list.
func mergeDependencies(groups ...sharedgodi.Dependencies) sharedgodi.Dependencies {
	var total int
	for _, group := range groups {
		total += len(group.List())
	}
	list := make([]sharedgodi.Dependency, 0, total)
	for _, group := range groups {
		list = append(list, group.List()...)
	}
	return sharedgodi.CollectDependencies(list...)
}

// ModuleBootstrap registers helpers to assemble bootstrap dependencies and server.
func ModuleBootstrap() sharedgodi.Dependencies {
	return mergeDependencies(
		ModuleBootstrapRoles(),
		ModuleBootstrapAuth(),
		sharedgodi.CollectDependencies(
			sharedgodi.NewDependency(provideSystemDependencies, sharedgodi.WithKey(KeyBootstrapSystemDeps)),
			sharedgodi.NewDependency(provideOutboxDependencies, sharedgodi.WithKey(KeyBootstrapOutboxDeps)),
			sharedgodi.NewDependency(provideAppRepositories, sharedgodi.WithKey(KeyBootstrapRepoDeps)),
			sharedgodi.NewDependency(provideUploadDependencies, sharedgodi.WithKey(KeyBootstrapUploadDeps)),
			sharedgodi.NewDependency(provideAdminServer, sharedgodi.WithKey(KeyBootstrapAdminServer)),
			sharedgodi.NewDependency(provideBootstrapDependencies, sharedgodi.WithKey(KeyBootstrapDeps)),
			sharedgodi.NewDependency(provideAdminPreviewAttach, sharedgodi.WithKey(KeyAdminPreviewAttachJob)),
			sharedgodi.NewDependency(provideAdminPreviewDetach, sharedgodi.WithKey(KeyAdminPreviewDetachJob)),
			sharedgodi.NewDependency(provideAdminNotificationJob, sharedgodi.WithKey(KeyAdminNotificationJob)),
			sharedgodi.NewDependency(provideAdminEventStream, sharedgodi.WithKey(KeyAdminEventStream)),
			sharedgodi.NewDependency(provideOutbox, sharedgodi.WithKey(KeyAdminOutboxRegister)),
			sharedgodi.NewDependency(provideAdminOutboxPutter, sharedgodi.WithKey(KeyAdminOutboxPutter)),
			sharedgodi.NewDependency(provideAdminRolesManager, sharedgodi.WithKey(KeyAdminRolesManager)),
			sharedgodi.NewDependency(provideAdminRolesUseCaseRepo, sharedgodi.WithKey(KeyAdminRolesUseCaseRepo)),
			sharedgodi.NewDependency(provideAdminUserRepo, sharedgodi.WithKey(KeyAdminUserRepo)),
			sharedgodi.NewDependency(provideAdminFileLoader, sharedgodi.WithKey(KeyAdminFileLoader)),
			sharedgodi.NewDependency(provideAdminFileRepo, sharedgodi.WithKey(KeyAdminFileRepo)),
			sharedgodi.NewDependency(provideAdminRepo, sharedgodi.WithKey(KeyAdminRepo)),
			sharedgodi.NewDependency(provideAdminNotificationRepo, sharedgodi.WithKey(KeyAdminNotificationRepo)),
			sharedgodi.NewDependency(provideAdminActionAuditRepo),
			sharedgodi.NewDependency(provideFirstAdminSetup),
			sharedgodi.NewDependency(provideAdminAuthService, sharedgodi.WithKey(KeyAdminAuthService)),
			sharedgodi.NewDependency(
				provideAdminJobsRepo,
				sharedgodi.WithKey(KeyAdminJobsRepo),
				sharedgodi.WithMatch(new(bootstrap.JobsRepo)),
				sharedgodi.WithMatch(new(outbox.JobsRepository)),
				sharedgodi.WithMatch(new(outbox.JobsStatRepository)),
			),
			sharedgodi.NewDependency(
				provideAdminJobsFailedRepo,
				sharedgodi.WithKey(KeyAdminJobsFailedRepo),
				sharedgodi.WithMatch(new(bootstrap.JobsFailedRepo)),
				sharedgodi.WithMatch(new(outbox.JobsFailedRepository)),
			),
		),
	)
}

// ModuleBootstrapAuth registers auth usecases.
func ModuleBootstrapAuth() sharedgodi.Dependencies {
	return sharedgodi.CollectDependencies(
		sharedgodi.NewDependency(provideAdminSessionStore, sharedgodi.WithKey(KeyAuthAdminSession)),
	)
}

// ModuleBootstrapRoles registers helpers to assemble bootstrap dependencies and server.
func ModuleBootstrapRoles() sharedgodi.Dependencies {
	return sharedgodi.CollectDependencies(
		sharedgodi.NewDependency(provideAuthAdapter),
		sharedgodi.NewDependency(provideRolesService, sharedgodi.WithKey(KeyRolesService)),
		sharedgodi.NewDependency(provideUseCaseListAllRoles, sharedgodi.WithKey(KeyRolesUseCaseListAll)),
		sharedgodi.NewDependency(provideUseCaseListSubjectRoles, sharedgodi.WithKey(KeyRolesUseCaseListSubject)),
		sharedgodi.NewDependency(provideRolesGuardService, sharedgodi.WithKey(KeyRolesGuardService)),
		sharedgodi.NewDependency(provideRolesGuardPermissionGuard, sharedgodi.WithKey(KeyRolesGuardPermissionGuard)),
		sharedgodi.NewDependency(provideRolesGuardRolesService, sharedgodi.WithKey(KeyRolesGuardRolesService)),
		sharedgodi.NewDependency(provideUseCaseListRoles, sharedgodi.WithKey(KeyRolesUseCaseList)),
		sharedgodi.NewDependency(provideUseCaseGetRole, sharedgodi.WithKey(KeyRolesUseCaseGet)),
		sharedgodi.NewDependency(provideUseCaseCreateRole, sharedgodi.WithKey(KeyRolesUseCaseCreate)),
		sharedgodi.NewDependency(provideUseCaseUpdateRole, sharedgodi.WithKey(KeyRolesUseCaseUpdate)),
		sharedgodi.NewDependency(provideUseCaseDeleteRole, sharedgodi.WithKey(KeyRolesUseCaseDelete)),
		sharedgodi.NewDependency(provideUseCaseSetRolePermissions, sharedgodi.WithKey(KeyRolesUseCaseSetPerms)),
		sharedgodi.NewDependency(provideUseCaseAssignSubjectRoles, sharedgodi.WithKey(KeyRolesUseCaseAssignSubject)),
		sharedgodi.NewDependency(provideUseCaseListPermissions, sharedgodi.WithKey(KeyRolesUseCaseListPerms)),
		sharedgodi.NewDependency(provideUseCaseListRolePermissions, sharedgodi.WithKey(KeyRolesUseCaseListRolePerms)),
	)
}
