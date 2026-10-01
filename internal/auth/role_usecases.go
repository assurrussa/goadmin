package auth

import (
	"context"
	"fmt"
	"strings"
)

type ListRolesRequest struct {
	IDs           []int64
	Slugs         []string
	IncludeSystem bool
	Search        string
	Limit         uint64
	Offset        uint64
}

type ListRolesRole struct {
	ID       int64  `json:"id"`
	UUID     string `json:"uuid"`
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	IsSystem bool   `json:"isSystem"`
}

type ListRolesResponse struct {
	Roles []ListRolesRole `json:"roles"`
}

type GetRoleRequest struct {
	RoleID             int64
	IncludePermissions bool
}

type GetRolePermission struct {
	Domain      PermissionDomain `json:"domain"`
	Action      PermissionAction `json:"action"`
	Description *string          `json:"description,omitempty"`
}

type GetRoleResponse struct {
	ID          int64               `json:"id"`
	UUID        string              `json:"uuid"`
	Slug        string              `json:"slug"`
	Name        string              `json:"name"`
	IsSystem    bool                `json:"isSystem"`
	Description *string             `json:"description,omitempty"`
	Permissions []GetRolePermission `json:"permissions,omitempty"`
}

type CreateRoleRequest struct {
	Slug           string
	Name           string
	Description    *string
	IsSystem       bool
	PermissionKeys []string
}

type CreateRoleResponse struct {
	RoleID int64  `json:"roleId"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
}

type UpdateRoleRequest struct {
	ID             int64
	Slug           string
	Name           string
	Description    *string
	IsSystem       *bool
	PermissionKeys *[]string
}

type UpdateRoleResponse struct {
	RoleID   int64  `json:"roleId"`
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	IsSystem bool   `json:"isSystem"`
}

type (
	DeleteRoleRequest  struct{ RoleID int64 }
	DeleteRoleResponse struct{}
)

type SetRolePermissionsRequest struct {
	RoleID         int64
	PermissionKeys []string
}

type SetRolePermissionsResponse struct {
	Total int `json:"total"`
}

type AssignSubjectRolesRequest struct {
	SubjectID string
	RoleIDs   []int64
}

type AssignSubjectRolesResponse struct {
	Assigned int `json:"assigned"`
}

type ListPermissionsRequest struct {
	Domain string
	Action string
}

type ListPermissionsPermission struct {
	ID          int64            `json:"id"`
	UUID        string           `json:"uuid"`
	Domain      PermissionDomain `json:"domain"`
	Action      PermissionAction `json:"action"`
	Description *string          `json:"description,omitempty"`
}

type ListPermissionsResponse struct {
	Permissions []ListPermissionsPermission `json:"permissions"`
}

type ListRolePermissionsRequest struct{ RoleID int64 }

type ListRolePermissionsPermission struct {
	Domain      PermissionDomain `json:"domain"`
	Action      PermissionAction `json:"action"`
	Description *string          `json:"description,omitempty"`
}

type ListRolePermissionsResponse struct {
	Permissions []ListRolePermissionsPermission `json:"permissions"`
}

type (
	ListSubjectRolesRequest  struct{ SubjectID string }
	ListSubjectRolesResponse struct {
		Roles []Role `json:"roles"`
	}
)

type (
	ListAllRolesRequest  struct{}
	ListAllRolesResponse struct {
		Roles           []Role
		Permissions     []Permission
		RolePermissions []RolePermission
		SubjectRoles    []SubjectRole
	}
)

type RoleUseCaseService interface {
	ListRoles(ctx context.Context, filter RoleFilter) ([]Role, error)
	GetRole(ctx context.Context, roleID int64) (*Role, error)
	CreateRole(ctx context.Context, input CreateRoleInput) (*Role, error)
	UpdateRole(ctx context.Context, input UpdateRoleInput) (*Role, error)
	DeleteRole(ctx context.Context, roleID int64) error
	SetRolePermissions(ctx context.Context, roleID int64, keys []PermissionKey) error
	AssignRolesToSubject(ctx context.Context, subjectID string, roleIDs []int64) error
	ListSubjectRoles(ctx context.Context, subjectID string) ([]Role, error)
	ListPermissions(ctx context.Context, filter PermissionFilter) ([]Permission, error)
	ListRolePermissions(ctx context.Context, roleID int64) ([]Permission, error)
}

type RoleSnapshotRepository interface {
	ListAllPermissions(ctx context.Context) ([]Permission, error)
	ListAllRolePermissions(ctx context.Context) ([]RolePermission, error)
	ListAllSubjectRoles(ctx context.Context) ([]SubjectRole, error)
}

type (
	ListRolesUseCase           struct{ service RoleUseCaseService }
	GetRoleUseCase             struct{ service RoleUseCaseService }
	CreateRoleUseCase          struct{ service RoleUseCaseService }
	UpdateRoleUseCase          struct{ service RoleUseCaseService }
	DeleteRoleUseCase          struct{ service RoleUseCaseService }
	SetRolePermissionsUseCase  struct{ service RoleUseCaseService }
	AssignSubjectRolesUseCase  struct{ service RoleUseCaseService }
	ListPermissionsUseCase     struct{ service RoleUseCaseService }
	ListRolePermissionsUseCase struct{ service RoleUseCaseService }
	ListSubjectRolesUseCase    struct{ service RoleUseCaseService }
	ListAllRolesUseCase        struct {
		service RoleUseCaseService
		repo    RoleSnapshotRepository
	}
)

func MustListRolesUseCase(service RoleUseCaseService) *ListRolesUseCase {
	return &ListRolesUseCase{service: requiredRoleService(service)}
}

func MustGetRoleUseCase(service RoleUseCaseService) *GetRoleUseCase {
	return &GetRoleUseCase{service: requiredRoleService(service)}
}

func MustCreateRoleUseCase(service RoleUseCaseService) *CreateRoleUseCase {
	return &CreateRoleUseCase{service: requiredRoleService(service)}
}

func MustUpdateRoleUseCase(service RoleUseCaseService) *UpdateRoleUseCase {
	return &UpdateRoleUseCase{service: requiredRoleService(service)}
}

func MustDeleteRoleUseCase(service RoleUseCaseService) *DeleteRoleUseCase {
	return &DeleteRoleUseCase{service: requiredRoleService(service)}
}

func MustSetRolePermissionsUseCase(service RoleUseCaseService) *SetRolePermissionsUseCase {
	return &SetRolePermissionsUseCase{service: requiredRoleService(service)}
}

func MustAssignSubjectRolesUseCase(service RoleUseCaseService) *AssignSubjectRolesUseCase {
	return &AssignSubjectRolesUseCase{service: requiredRoleService(service)}
}

func MustListPermissionsUseCase(service RoleUseCaseService) *ListPermissionsUseCase {
	return &ListPermissionsUseCase{service: requiredRoleService(service)}
}

func MustListRolePermissionsUseCase(service RoleUseCaseService) *ListRolePermissionsUseCase {
	return &ListRolePermissionsUseCase{service: requiredRoleService(service)}
}

func MustListSubjectRolesUseCase(service RoleUseCaseService) *ListSubjectRolesUseCase {
	return &ListSubjectRolesUseCase{service: requiredRoleService(service)}
}

func MustListAllRolesUseCase(
	service RoleUseCaseService,
	repo RoleSnapshotRepository,
) *ListAllRolesUseCase {
	if repo == nil {
		panic("goadmin roles snapshot repository is required")
	}

	return &ListAllRolesUseCase{service: requiredRoleService(service), repo: repo}
}

func requiredRoleService(service RoleUseCaseService) RoleUseCaseService {
	if service == nil {
		panic("goadmin roles service is required")
	}

	return service
}

func (u *ListRolesUseCase) Handle(ctx context.Context, request ListRolesRequest) (ListRolesResponse, error) {
	if err := validateIDs(request.IDs); err != nil {
		return ListRolesResponse{}, err
	}
	roles, err := u.service.ListRoles(ctx, RoleFilter{
		IDs: request.IDs, Slugs: request.Slugs, IncludeSys: request.IncludeSystem,
		Search: request.Search, Pagination: Pagination{Limit: request.Limit, Offset: request.Offset},
	})
	if err != nil {
		return ListRolesResponse{}, err
	}
	response := ListRolesResponse{Roles: make([]ListRolesRole, 0, len(roles))}
	for _, role := range roles {
		response.Roles = append(response.Roles, ListRolesRole{
			ID: role.ID, UUID: role.UUID, Slug: role.Slug, Name: role.Name, IsSystem: role.IsSystem,
		})
	}

	return response, nil
}

func (u *GetRoleUseCase) Handle(ctx context.Context, request GetRoleRequest) (GetRoleResponse, error) {
	if request.RoleID <= 0 {
		return GetRoleResponse{}, ErrInvalidRoleID
	}
	role, err := u.service.GetRole(ctx, request.RoleID)
	if err != nil {
		return GetRoleResponse{}, err
	}
	response := GetRoleResponse{
		ID: role.ID, UUID: role.UUID, Slug: role.Slug, Name: role.Name, IsSystem: role.IsSystem,
	}
	if role.Description.Valid {
		description := role.Description.String
		response.Description = &description
	}
	if request.IncludePermissions {
		permissions, err := u.service.ListRolePermissions(ctx, role.ID)
		if err != nil {
			return GetRoleResponse{}, err
		}
		response.Permissions = make([]GetRolePermission, 0, len(permissions))
		for _, permission := range permissions {
			response.Permissions = append(response.Permissions, GetRolePermission{
				Domain: permission.Domain, Action: permission.Action, Description: permission.Description,
			})
		}
	}

	return response, nil
}

func (u *CreateRoleUseCase) Handle(ctx context.Context, request CreateRoleRequest) (CreateRoleResponse, error) {
	if strings.TrimSpace(request.Slug) == "" {
		return CreateRoleResponse{}, ErrInvalidRoleSlug
	}
	if strings.TrimSpace(request.Name) == "" {
		return CreateRoleResponse{}, ErrInvalidRoleName
	}
	keys, err := parsePermissionKeys(request.PermissionKeys)
	if err != nil {
		return CreateRoleResponse{}, err
	}
	role, err := u.service.CreateRole(ctx, CreateRoleInput{
		Slug: request.Slug, Name: request.Name, Description: request.Description,
		IsSystem: request.IsSystem, Permissions: keys,
	})
	if err != nil {
		return CreateRoleResponse{}, err
	}

	return CreateRoleResponse{RoleID: role.ID, Slug: role.Slug, Name: role.Name}, nil
}

func (u *UpdateRoleUseCase) Handle(ctx context.Context, request UpdateRoleRequest) (UpdateRoleResponse, error) {
	if request.ID <= 0 {
		return UpdateRoleResponse{}, ErrInvalidRoleID
	}
	if strings.TrimSpace(request.Slug) == "" || strings.TrimSpace(request.Name) == "" {
		return UpdateRoleResponse{}, ErrInvalidRoleName
	}
	var keys *[]PermissionKey
	if request.PermissionKeys != nil {
		parsed, err := parsePermissionKeys(*request.PermissionKeys)
		if err != nil {
			return UpdateRoleResponse{}, err
		}
		keys = &parsed
	}
	role, err := u.service.UpdateRole(ctx, UpdateRoleInput{
		ID: request.ID, Slug: request.Slug, Name: request.Name, Description: request.Description,
		IsSystem: request.IsSystem, Permissions: keys,
	})
	if err != nil {
		return UpdateRoleResponse{}, err
	}

	return UpdateRoleResponse{
		RoleID: role.ID, Slug: role.Slug, Name: role.Name, IsSystem: role.IsSystem,
	}, nil
}

func (u *DeleteRoleUseCase) Handle(
	ctx context.Context,
	request DeleteRoleRequest,
) (DeleteRoleResponse, error) {
	if request.RoleID <= 0 {
		return DeleteRoleResponse{}, ErrInvalidRoleID
	}

	return DeleteRoleResponse{}, u.service.DeleteRole(ctx, request.RoleID)
}

func (u *SetRolePermissionsUseCase) Handle(
	ctx context.Context,
	request SetRolePermissionsRequest,
) (SetRolePermissionsResponse, error) {
	if request.RoleID <= 0 {
		return SetRolePermissionsResponse{}, ErrInvalidRoleID
	}
	keys, err := parsePermissionKeys(request.PermissionKeys)
	if err != nil {
		return SetRolePermissionsResponse{}, err
	}
	if err := u.service.SetRolePermissions(ctx, request.RoleID, keys); err != nil {
		return SetRolePermissionsResponse{}, err
	}

	return SetRolePermissionsResponse{Total: len(keys)}, nil
}

func (u *AssignSubjectRolesUseCase) Handle(
	ctx context.Context,
	request AssignSubjectRolesRequest,
) (AssignSubjectRolesResponse, error) {
	if strings.TrimSpace(request.SubjectID) == "" {
		return AssignSubjectRolesResponse{}, ErrInvalidSubjectID
	}
	if err := validateIDs(request.RoleIDs); err != nil {
		return AssignSubjectRolesResponse{}, err
	}
	if err := u.service.AssignRolesToSubject(ctx, request.SubjectID, request.RoleIDs); err != nil {
		return AssignSubjectRolesResponse{}, err
	}

	return AssignSubjectRolesResponse{Assigned: len(request.RoleIDs)}, nil
}

func (u *ListPermissionsUseCase) Handle(
	ctx context.Context,
	request ListPermissionsRequest,
) (ListPermissionsResponse, error) {
	permissions, err := u.service.ListPermissions(ctx, PermissionFilter{
		Domain: request.Domain, Action: request.Action,
	})
	if err != nil {
		return ListPermissionsResponse{}, err
	}
	response := ListPermissionsResponse{Permissions: make([]ListPermissionsPermission, 0, len(permissions))}
	for _, permission := range permissions {
		response.Permissions = append(response.Permissions, ListPermissionsPermission{
			ID: permission.ID, UUID: permission.UUID, Domain: permission.Domain,
			Action: permission.Action, Description: permission.Description,
		})
	}

	return response, nil
}

func (u *ListRolePermissionsUseCase) Handle(
	ctx context.Context,
	request ListRolePermissionsRequest,
) (ListRolePermissionsResponse, error) {
	if request.RoleID <= 0 {
		return ListRolePermissionsResponse{}, ErrInvalidRoleID
	}
	permissions, err := u.service.ListRolePermissions(ctx, request.RoleID)
	if err != nil {
		return ListRolePermissionsResponse{}, err
	}
	response := ListRolePermissionsResponse{
		Permissions: make([]ListRolePermissionsPermission, 0, len(permissions)),
	}
	for _, permission := range permissions {
		response.Permissions = append(response.Permissions, ListRolePermissionsPermission{
			Domain: permission.Domain, Action: permission.Action, Description: permission.Description,
		})
	}

	return response, nil
}

func (u *ListSubjectRolesUseCase) Handle(
	ctx context.Context,
	request ListSubjectRolesRequest,
) (ListSubjectRolesResponse, error) {
	if strings.TrimSpace(request.SubjectID) == "" {
		return ListSubjectRolesResponse{}, ErrInvalidSubjectID
	}
	roles, err := u.service.ListSubjectRoles(ctx, request.SubjectID)
	if err != nil {
		return ListSubjectRolesResponse{}, err
	}

	return ListSubjectRolesResponse{Roles: roles}, nil
}

func (u *ListAllRolesUseCase) Handle(
	ctx context.Context,
	_ ListAllRolesRequest,
) (ListAllRolesResponse, error) {
	roles, err := u.service.ListRoles(ctx, RoleFilter{IncludeSys: true})
	if err != nil {
		return ListAllRolesResponse{}, err
	}
	permissions, err := u.repo.ListAllPermissions(ctx)
	if err != nil {
		return ListAllRolesResponse{}, err
	}
	rolePermissions, err := u.repo.ListAllRolePermissions(ctx)
	if err != nil {
		return ListAllRolesResponse{}, err
	}
	subjectRoles, err := u.repo.ListAllSubjectRoles(ctx)
	if err != nil {
		return ListAllRolesResponse{}, err
	}

	return ListAllRolesResponse{
		Roles: roles, Permissions: permissions, RolePermissions: rolePermissions,
		SubjectRoles: subjectRoles,
	}, nil
}

func parsePermissionKeys(values []string) ([]PermissionKey, error) {
	keys := make([]PermissionKey, 0, len(values))
	for _, value := range values {
		key := ParsePermissionKey(value)
		if key.IsZero() {
			return nil, fmt.Errorf("%w: %q", ErrInvalidPermission, value)
		}
		keys = append(keys, key)
	}

	return keys, nil
}

func validateIDs(values []int64) error {
	for _, value := range values {
		if value <= 0 {
			return ErrInvalidRoleID
		}
	}

	return nil
}
