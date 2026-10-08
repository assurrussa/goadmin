package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/rbac"
	logger "github.com/assurrussa/gologger"
)

type RoleService struct {
	service         *rbac.Service
	roleTransaction func(context.Context, []goauth.SubjectID, func(context.Context) error) error
}

// Service is retained as a host-local name while goadmin handlers migrate;
// it is not a public goauth alias.
type Service = RoleService

func NewRoleService(service *rbac.Service) *RoleService {
	return &RoleService{service: service}
}

func (s *RoleService) ListRoles(ctx context.Context, filter RoleFilter) ([]Role, error) {
	if s == nil || s.service == nil {
		return nil, errors.New("goadmin roles service is required")
	}
	values, err := s.service.Roles(ctx, rbac.RoleFilter{
		IDs:           filter.IDs,
		Slugs:         filter.Slugs,
		IncludeSystem: filter.IncludeSys,
		Search:        filter.Search,
		Limit:         filter.Pagination.Limit,
		Offset:        filter.Pagination.Offset,
	})
	if err != nil {
		return nil, err
	}
	roles := make([]Role, 0, len(values))
	for _, value := range values {
		roles = append(roles, roleFromRBAC(value))
	}

	return roles, nil
}

func (s *RoleService) GetRole(ctx context.Context, roleID int64) (*Role, error) {
	if roleID <= 0 {
		return nil, ErrInvalidRoleID
	}
	value, err := s.service.Role(ctx, roleID)
	if err != nil {
		return nil, err
	}
	role := roleFromRBAC(value)

	return &role, nil
}

func (s *RoleService) CreateRole(ctx context.Context, input CreateRoleInput) (*Role, error) {
	keys, err := rbacPermissionKeys(input.Permissions)
	if err != nil {
		return nil, err
	}
	description := ""
	if input.Description != nil {
		description = *input.Description
	}
	value, err := s.service.CreateRole(ctx, rbac.Role{
		Slug: input.Slug, Name: input.Name, Description: description, System: input.IsSystem,
	}, keys)
	if err != nil {
		return nil, err
	}
	role := roleFromRBAC(value)

	return &role, nil
}

func (s *RoleService) UpdateRole(ctx context.Context, input UpdateRoleInput) (*Role, error) {
	current, err := s.service.Role(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	current.Slug = input.Slug
	current.Name = input.Name
	if input.Description == nil {
		current.Description = ""
	} else {
		current.Description = *input.Description
	}
	if input.IsSystem != nil {
		current.System = *input.IsSystem
	}
	var keys *[]rbac.PermissionKey
	if input.Permissions != nil {
		converted, conversionErr := rbacPermissionKeys(*input.Permissions)
		if conversionErr != nil {
			return nil, conversionErr
		}
		keys = &converted
	}
	value, err := s.service.UpdateRole(ctx, current, keys)
	if err != nil {
		return nil, err
	}
	role := roleFromRBAC(value)

	return &role, nil
}

func (s *RoleService) DeleteRole(ctx context.Context, roleID int64) error {
	return s.service.DeleteRole(ctx, roleID)
}

func (s *RoleService) ListRolePermissions(ctx context.Context, roleID int64) ([]Permission, error) {
	values, err := s.service.RolePermissions(ctx, roleID)
	if err != nil {
		return nil, err
	}

	return mapPermissions(values)
}

func (s *RoleService) SetRolePermissions(ctx context.Context, roleID int64, keys []PermissionKey) error {
	converted, err := rbacPermissionKeys(keys)
	if err != nil {
		return err
	}

	return s.service.ReplaceRolePermissions(ctx, roleID, converted)
}

func (s *RoleService) AssignRolesToSubject(ctx context.Context, subjectID string, roleIDs []int64) error {
	parsed, err := goauth.ParseSubjectID(subjectID)
	if err != nil {
		return ErrInvalidSubjectID
	}

	return s.service.ReplaceSubjectRoles(ctx, parsed, roleIDs)
}

func (s *RoleService) ListSubjectRoles(ctx context.Context, subjectID string) ([]Role, error) {
	parsed, err := goauth.ParseSubjectID(subjectID)
	if err != nil {
		return nil, ErrInvalidSubjectID
	}
	values, err := s.service.SubjectRoles(ctx, parsed)
	if err != nil {
		return nil, err
	}
	roles := make([]Role, 0, len(values))
	for _, value := range values {
		roles = append(roles, roleFromRBAC(value))
	}

	return roles, nil
}

func (s *RoleService) ListPermissions(ctx context.Context, filter PermissionFilter) ([]Permission, error) {
	keys, err := rbacPermissionKeys(filter.Keys)
	if err != nil {
		return nil, err
	}
	values, err := s.service.Permissions(ctx, rbac.PermissionFilter{
		IDs: filter.IDs, Keys: keys, Domain: filter.Domain, Action: filter.Action,
	})
	if err != nil {
		return nil, err
	}

	return mapPermissions(values)
}

func (s *RoleService) EnsurePermissions(ctx context.Context, inputs []CreatePermissionInput) error {
	_, err := s.CreatePermissions(ctx, inputs)

	return err
}

func (s *RoleService) CreatePermissions(ctx context.Context, inputs []CreatePermissionInput) ([]Permission, error) {
	permissions := make([]Permission, 0, len(inputs))
	for _, input := range inputs {
		key, err := input.Key.rbac()
		if err != nil {
			return nil, ErrInvalidPermission
		}
		value, err := s.service.UpsertPermission(ctx, rbac.Permission{
			Key: key, Description: strings.TrimSpace(input.Description),
		})
		if err != nil {
			return nil, err
		}
		permission, err := permissionFromRBAC(value)
		if err != nil {
			return nil, err
		}
		permissions = append(permissions, permission)
	}

	return permissions, nil
}

func (s *RoleService) ListAllPermissionsByRoles(
	ctx context.Context,
	roleIDs []int64,
) ([]PermissionWithRole, error) {
	result := make([]PermissionWithRole, 0)
	for _, roleID := range roleIDs {
		permissions, err := s.ListRolePermissions(ctx, roleID)
		if err != nil {
			return nil, err
		}
		for _, permission := range permissions {
			result = append(result, PermissionWithRole{RoleID: roleID, Permission: permission})
		}
	}

	return result, nil
}

func (s *RoleService) HasPermission(ctx context.Context, subjectID string, key PermissionKey) (bool, error) {
	parsed, err := goauth.ParseSubjectID(subjectID)
	if err != nil {
		return false, ErrInvalidSubjectID
	}
	permission, err := key.rbac()
	if err != nil {
		return false, ErrInvalidPermission
	}

	return s.service.Can(ctx, parsed, permission), nil
}

func (s *RoleService) ListAllPermissions(ctx context.Context) ([]Permission, error) {
	return s.ListPermissions(ctx, PermissionFilter{})
}

func (s *RoleService) ListAllRolePermissions(ctx context.Context) ([]RolePermission, error) {
	snapshot, err := s.service.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]RolePermission, 0, len(snapshot.RolePermissions))
	for _, assignment := range snapshot.RolePermissions {
		result = append(result, RolePermission{
			RoleID: assignment.RoleID, PermissionID: assignment.PermissionID,
		})
	}

	return result, nil
}

func (s *RoleService) ListAllSubjectRoles(ctx context.Context) ([]SubjectRole, error) {
	snapshot, err := s.service.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]SubjectRole, 0, len(snapshot.SubjectRoles))
	for _, assignment := range snapshot.SubjectRoles {
		result = append(result, SubjectRole{
			SubjectID: assignment.SubjectID.String(), RoleID: assignment.RoleID,
		})
	}

	return result, nil
}

func rbacPermissionKeys(keys []PermissionKey) ([]rbac.PermissionKey, error) {
	result := make([]rbac.PermissionKey, 0, len(keys))
	for _, key := range keys {
		converted, err := key.rbac()
		if err != nil {
			return nil, ErrInvalidPermission
		}
		result = append(result, converted)
	}

	return result, nil
}

func mapPermissions(values []rbac.Permission) ([]Permission, error) {
	permissions := make([]Permission, 0, len(values))
	for _, value := range values {
		permission, err := permissionFromRBAC(value)
		if err != nil {
			return nil, err
		}
		permissions = append(permissions, permission)
	}

	return permissions, nil
}

type guardRolesUseCase interface {
	Handle(ctx context.Context, request ListSubjectRolesRequest) (ListSubjectRolesResponse, error)
}

type guardPermissions interface {
	ListAllPermissionsByRoles(ctx context.Context, roleIDs []int64) ([]PermissionWithRole, error)
	HasPermission(ctx context.Context, subjectID string, key PermissionKey) (bool, error)
}

type guardCache interface {
	Enabled(ctx context.Context) bool
	SubjectRoles(subjectID string) []Role
	AllRolePermissions(roles []Role) map[int64]map[int64]Permission
}

type GuardServiceOptions struct {
	rolesUseCase      guardRolesUseCase
	permissionUseCase guardPermissions
	cache             guardCache
	logger            logger.Logger
}

func NewGuardServiceOptions(
	rolesUseCase guardRolesUseCase,
	permissionUseCase guardPermissions,
	cache guardCache,
	log logger.Logger,
) GuardServiceOptions {
	return GuardServiceOptions{
		rolesUseCase: rolesUseCase, permissionUseCase: permissionUseCase, cache: cache, logger: log,
	}
}

type GuardService struct {
	rolesUseCase      guardRolesUseCase
	permissionUseCase guardPermissions
	cache             guardCache
	logger            logger.Logger
}

func NewGuardService(
	rolesUseCase *ListSubjectRolesUseCase,
	roles *RoleService,
	log logger.Logger,
) (*GuardService, error) {
	return NewGuardServiceWithOptions(NewGuardServiceOptions(rolesUseCase, roles, disabledRoleCache{}, log))
}

func NewGuardServiceWithOptions(options GuardServiceOptions) (*GuardService, error) {
	if options.rolesUseCase == nil || options.permissionUseCase == nil || options.cache == nil {
		return nil, errors.New("goadmin roles guard dependencies are required")
	}
	if options.logger == nil {
		options.logger = logger.Discard()
	}

	return &GuardService{
		rolesUseCase: options.rolesUseCase, permissionUseCase: options.permissionUseCase,
		cache: options.cache, logger: options.logger,
	}, nil
}

func (s *GuardService) SubjectGuardCheck(
	ctx context.Context,
	subjectID string,
	key PermissionKey,
	options ...PermissionGuardOption,
) bool {
	roles, err := s.GetRolesSubject(ctx, subjectID)
	if err != nil {
		return false
	}
	for _, role := range roles {
		for _, bypass := range CreateRoles(options...) {
			if strings.EqualFold(role.Slug, bypass) {
				return true
			}
		}
	}
	if s.cache.Enabled(ctx) {
		for _, permissions := range s.cache.AllRolePermissions(roles) {
			for _, permission := range permissions {
				if permission.Domain == key.Domain && permission.Action == key.Action {
					return true
				}
			}
		}

		return false
	}
	allowed, err := s.permissionUseCase.HasPermission(ctx, subjectID, key)

	return err == nil && allowed
}

func (s *GuardService) IsSuperSubject(ctx context.Context, subjectID string) bool {
	roles, err := s.GetRolesSubject(ctx, subjectID)
	if err != nil {
		return false
	}
	for _, role := range roles {
		if role.Slug == SuperAdminRole {
			return true
		}
	}

	return false
}

func (s *GuardService) SubjectCan(
	ctx context.Context,
	subjectID string,
	domain PermissionDomain,
	action PermissionAction,
) bool {
	return s.SubjectGuardCheck(ctx, subjectID, NewPermissionKey(domain, action))
}

func (s *GuardService) GetRolesSubject(ctx context.Context, subjectID string) ([]Role, error) {
	if strings.TrimSpace(subjectID) == "" {
		return nil, ErrInvalidSubjectID
	}
	if s.cache.Enabled(ctx) {
		roles := s.cache.SubjectRoles(subjectID)
		if len(roles) == 0 {
			return nil, ErrRoleNotFound
		}

		return roles, nil
	}
	response, err := s.rolesUseCase.Handle(ctx, ListSubjectRolesRequest{SubjectID: subjectID})
	if err != nil {
		return nil, err
	}

	return response.Roles, nil
}

func (s *GuardService) GetPermissions(
	ctx context.Context,
	roles []Role,
) (map[int64]map[int64]Permission, error) {
	if s.cache.Enabled(ctx) {
		return s.cache.AllRolePermissions(roles), nil
	}
	roleIDs := make([]int64, 0, len(roles))
	for _, role := range roles {
		roleIDs = append(roleIDs, role.ID)
	}
	values, err := s.permissionUseCase.ListAllPermissionsByRoles(ctx, roleIDs)
	if err != nil {
		return nil, fmt.Errorf("list role permissions: %w", err)
	}
	result := make(map[int64]map[int64]Permission)
	for _, value := range values {
		if result[value.RoleID] == nil {
			result[value.RoleID] = make(map[int64]Permission)
		}
		result[value.RoleID][value.ID] = value.Permission
	}

	return result, nil
}

type disabledRoleCache struct{}

func (disabledRoleCache) Enabled(context.Context) bool { return false }
func (disabledRoleCache) SubjectRoles(string) []Role   { return nil }
func (disabledRoleCache) AllRolePermissions([]Role) map[int64]map[int64]Permission {
	return nil
}
