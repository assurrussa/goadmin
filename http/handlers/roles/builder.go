package roles

import (
	"context"
	"errors"

	integrationroles "github.com/assurrussa/goadmin/internal/auth"
)

var (
	ErrUseCaseServiceNil = errors.New("roles handler: usecase service is nil")
	ErrUseCaseRepoNil    = errors.New("roles handler: usecase repository is nil")
	ErrAdminRepoNil      = errors.New("roles handler: admin repository is nil")
	errInvalidAdminID    = errors.New("invalid admin id")
)

type UseCaseService interface {
	ListRoles(ctx context.Context, filter integrationroles.RoleFilter) ([]integrationroles.Role, error)
	GetRole(ctx context.Context, id int64) (*integrationroles.Role, error)
	CreateRole(ctx context.Context, input integrationroles.CreateRoleInput) (*integrationroles.Role, error)
	UpdateRole(ctx context.Context, input integrationroles.UpdateRoleInput) (*integrationroles.Role, error)
	DeleteRole(ctx context.Context, id int64) error
	SetRolePermissions(ctx context.Context, roleID int64, keys []integrationroles.PermissionKey) error
	AssignRolesToSubject(ctx context.Context, subjectID string, roleIDs []int64) error
	ListSubjectRoles(ctx context.Context, subjectID string) ([]integrationroles.Role, error)
	ListPermissions(ctx context.Context, filter integrationroles.PermissionFilter) ([]integrationroles.Permission, error)
	ListRolePermissions(ctx context.Context, roleID int64) ([]integrationroles.Permission, error)
}

type UseCaseRepository interface {
	ListAllRolePermissions(ctx context.Context) ([]integrationroles.RolePermission, error)
	ListAllSubjectRoles(ctx context.Context) ([]integrationroles.SubjectRole, error)
	ListAllPermissions(ctx context.Context) ([]integrationroles.Permission, error)
	ListSubjectRoles(ctx context.Context, subjectID string) ([]integrationroles.Role, error)
}

func BuildUseCases(service UseCaseService, repo UseCaseRepository, adminRepo adminRepository) (UseCases, error) {
	if service == nil {
		return UseCases{}, ErrUseCaseServiceNil
	}
	if repo == nil {
		return UseCases{}, ErrUseCaseRepoNil
	}
	if adminRepo == nil {
		return UseCases{}, ErrAdminRepoNil
	}

	assignSubjectRoles := integrationroles.MustAssignSubjectRolesUseCase(service)
	listSubjectRoles := integrationroles.MustListSubjectRolesUseCase(service)

	return UseCases{
		ListRoles:           integrationroles.MustListRolesUseCase(service),
		GetRole:             integrationroles.MustGetRoleUseCase(service),
		CreateRole:          integrationroles.MustCreateRoleUseCase(service),
		UpdateRole:          integrationroles.MustUpdateRoleUseCase(service),
		DeleteRole:          integrationroles.MustDeleteRoleUseCase(service),
		SetPermissions:      integrationroles.MustSetRolePermissionsUseCase(service),
		AssignAdminRoles:    newAssignAdminRolesAdapter(adminRepo, assignSubjectRoles),
		ListPermissions:     integrationroles.MustListPermissionsUseCase(service),
		ListRolePermissions: integrationroles.MustListRolePermissionsUseCase(service),
		ListAdminRoles:      newListAdminRolesAdapter(adminRepo, listSubjectRoles),
		ListAllRoles:        integrationroles.MustListAllRolesUseCase(service, repo),
	}, nil
}
