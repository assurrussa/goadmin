package auth

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/assurrussa/goauth/rbac"
)

var (
	ErrRoleNotFound        = rbac.ErrRoleNotFound
	ErrPermissionNotFound  = rbac.ErrPermissionNotFound
	ErrInvalidRoleID       = errors.New("invalid role id")
	ErrInvalidRoleSlug     = errors.New("invalid role slug")
	ErrInvalidRoleName     = errors.New("invalid role name")
	ErrInvalidPermission   = rbac.ErrInvalidPermissionKey
	ErrSystemRoleProtected = rbac.ErrSystemRoleProtected
)

const (
	SuperAdminRole   = "super_admin"
	SuperSubjectRole = SuperAdminRole
)

type (
	PermissionDomain string
	PermissionAction string
)

func (p PermissionDomain) String() string { return string(p) }
func (p PermissionAction) String() string { return string(p) }

const (
	PermissionDomainRoles       PermissionDomain = "roles"
	PermissionDomainPermissions PermissionDomain = "permissions"
	PermissionDomainAdmins      PermissionDomain = "admins"
	PermissionDomainDashboard   PermissionDomain = "dashboard"
	PermissionDomainOperations  PermissionDomain = "operations"
	PermissionDomainUsers       PermissionDomain = "users"
	PermissionDomainUploads     PermissionDomain = "uploads"
	PermissionDomainQueues      PermissionDomain = "queues"
)

const (
	PermissionActionRead   PermissionAction = "read"
	PermissionActionCreate PermissionAction = "create"
	PermissionActionUpdate PermissionAction = "update"
	PermissionActionDelete PermissionAction = "delete"
	PermissionActionAssign PermissionAction = "assign"
	PermissionActionSync   PermissionAction = "sync"
)

type PermissionKey struct {
	Domain PermissionDomain
	Action PermissionAction
}

func NewPermissionKey(domain PermissionDomain, action PermissionAction) PermissionKey {
	return PermissionKey{Domain: domain, Action: action}
}

func ParsePermissionKey(value string) PermissionKey {
	domain, action, ok := strings.Cut(strings.TrimSpace(value), ":")
	if !ok || strings.Contains(action, ":") {
		return PermissionKey{}
	}
	key := PermissionKey{Domain: PermissionDomain(domain), Action: PermissionAction(action)}
	if _, err := key.rbac(); err != nil {
		return PermissionKey{}
	}

	return key
}

func (k PermissionKey) String() string { return string(k.Domain) + ":" + string(k.Action) }
func (k PermissionKey) IsZero() bool   { return k.Domain == "" || k.Action == "" }

func (k PermissionKey) rbac() (rbac.PermissionKey, error) {
	return rbac.NewPermissionKey(string(k.Domain), string(k.Action))
}

type PermissionDefinition struct {
	Key         PermissionKey
	Description string
}

type PermissionCatalog struct {
	Data []PermissionDefinition
}

func DefaultPermissionDefinitions() PermissionCatalog {
	return NewPermissionCatalog(defaultPermissionDefinitions)
}

func NewPermissionCatalog(definitions []PermissionDefinition) PermissionCatalog {
	return PermissionCatalog{Data: ClonePermissionDefinitions(definitions)}
}

func ClonePermissionDefinitions(definitions []PermissionDefinition) []PermissionDefinition {
	if len(definitions) == 0 {
		return nil
	}

	return slices.Clone(definitions)
}

func MergePermissionDefinitions(definitionSets ...[]PermissionDefinition) []PermissionDefinition {
	var merged []PermissionDefinition
	indexByKey := make(map[string]int)
	for _, definitions := range definitionSets {
		for _, definition := range definitions {
			if definition.Key.IsZero() {
				continue
			}
			key := definition.Key.String()
			if index, found := indexByKey[key]; found {
				if strings.TrimSpace(definition.Description) != "" {
					merged[index].Description = definition.Description
				}
				continue
			}
			indexByKey[key] = len(merged)
			merged = append(merged, definition)
		}
	}

	return merged
}

func (p PermissionCatalog) GetPermissions() []PermissionKey {
	permissions := make([]PermissionKey, 0, len(p.Data))
	for _, definition := range p.Data {
		permissions = append(permissions, definition.Key)
	}

	return permissions
}

func (p PermissionCatalog) Get(domain PermissionDomain, action PermissionAction) (PermissionDefinition, error) {
	for _, definition := range p.Data {
		if definition.Key.Domain == domain && definition.Key.Action == action {
			return definition, nil
		}
	}

	return PermissionDefinition{}, ErrPermissionNotFound
}

type PermissionGuardConfig struct {
	bypassRoles []string
}

type PermissionGuardOption func(*PermissionGuardConfig)

func WithBypassRoles(slugs ...string) PermissionGuardOption {
	return func(config *PermissionGuardConfig) {
		config.bypassRoles = append(config.bypassRoles, slugs...)
	}
}

func CreateRoles(options ...PermissionGuardOption) []string {
	config := &PermissionGuardConfig{bypassRoles: []string{SuperSubjectRole}}
	for _, option := range options {
		option(config)
	}

	return slices.Clone(config.bypassRoles)
}

type Role struct {
	ID          int64          `json:"id" db:"id"`
	UUID        string         `json:"uuid" db:"uuid"`
	Slug        string         `json:"slug" db:"slug"`
	Name        string         `json:"name" db:"name"`
	Description sql.NullString `json:"description" db:"description"`
	IsSystem    bool           `json:"isSystem" db:"is_system"`
	CreatedAt   time.Time      `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time      `json:"updatedAt" db:"updated_at"`
}

func (r Role) HasDescription() bool { return r.Description.Valid && r.Description.String != "" }

type Permission struct {
	ID          int64            `json:"id" db:"id"`
	UUID        string           `json:"uuid" db:"uuid"`
	Domain      PermissionDomain `json:"domain" db:"domain"`
	Action      PermissionAction `json:"action" db:"action"`
	Description *string          `json:"description" db:"description"`
	CreatedAt   time.Time        `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time        `json:"updatedAt" db:"updated_at"`
}

func (p Permission) Key() string { return NewPermissionKey(p.Domain, p.Action).String() }

type PermissionWithRole struct {
	RoleID int64 `json:"roleId" db:"role_id"`
	Permission
}

type RolePermission struct {
	RoleID       int64     `json:"roleId" db:"role_id"`
	PermissionID int64     `json:"permissionId" db:"permission_id"`
	CreatedAt    time.Time `json:"createdAt" db:"created_at"`
}

type SubjectRole struct {
	SubjectID string    `json:"subjectId" db:"subject_id"`
	RoleID    int64     `json:"roleId" db:"role_id"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
}

type RoleFilter struct {
	IDs        []int64
	Slugs      []string
	IncludeSys bool
	Search     string
	Pagination Pagination
}

type PermissionFilter struct {
	IDs    []int64
	UUIDs  []string
	Keys   []PermissionKey
	Domain string
	Action string
}

type Pagination struct {
	Limit  uint64
	Offset uint64
}

type CreatePermissionInput struct {
	Key         PermissionKey
	Description string
}

type CreateRoleInput struct {
	Slug        string
	Name        string
	Description *string
	IsSystem    bool
	Permissions []PermissionKey
}

type UpdateRoleInput struct {
	ID          int64
	Slug        string
	Name        string
	Description *string
	IsSystem    *bool
	Permissions *[]PermissionKey
}

func roleFromRBAC(value rbac.Role) Role {
	role := Role{
		ID:        value.ID,
		UUID:      value.PublicID,
		Slug:      value.Slug,
		Name:      value.Name,
		IsSystem:  value.System,
		CreatedAt: value.CreatedAt,
		UpdatedAt: value.UpdatedAt,
	}
	if value.Description != "" {
		role.Description = sql.NullString{String: value.Description, Valid: true}
	}

	return role
}

func permissionFromRBAC(value rbac.Permission) (Permission, error) {
	key := ParsePermissionKey(string(value.Key))
	if key.IsZero() {
		return Permission{}, fmt.Errorf("map RBAC permission %q: %w", value.Key, ErrInvalidPermission)
	}
	permission := Permission{
		ID:        value.ID,
		UUID:      value.PublicID,
		Domain:    key.Domain,
		Action:    key.Action,
		CreatedAt: value.CreatedAt,
		UpdatedAt: value.UpdatedAt,
	}
	if value.Description != "" {
		description := value.Description
		permission.Description = &description
	}

	return permission, nil
}

var defaultPermissionDefinitions = []PermissionDefinition{
	{Key: NewPermissionKey(PermissionDomainRoles, PermissionActionRead), Description: "Просмотр списка ролей и деталей"},
	{Key: NewPermissionKey(PermissionDomainRoles, PermissionActionCreate), Description: "Создание новой роли"},
	{Key: NewPermissionKey(PermissionDomainRoles, PermissionActionUpdate), Description: "Редактирование существующей роли"},
	{Key: NewPermissionKey(PermissionDomainRoles, PermissionActionDelete), Description: "Удаление роли"},
	{Key: NewPermissionKey(PermissionDomainRoles, PermissionActionAssign), Description: "Назначение ролей администраторам"},
	{Key: NewPermissionKey(PermissionDomainPermissions, PermissionActionRead), Description: "Просмотр справочника разрешений"},
	{
		Key:         NewPermissionKey(PermissionDomainPermissions, PermissionActionSync),
		Description: "Синхронизация справочника разрешений с кодовой базой",
	},
	{Key: NewPermissionKey(PermissionDomainDashboard, PermissionActionRead), Description: "Просмотр dashboard"},
	{Key: NewPermissionKey(PermissionDomainAdmins, PermissionActionRead), Description: "Просмотр админов"},
	{Key: NewPermissionKey(PermissionDomainAdmins, PermissionActionCreate), Description: "Создание нового админа"},
	{Key: NewPermissionKey(PermissionDomainAdmins, PermissionActionUpdate), Description: "Редактирование админа"},
	{Key: NewPermissionKey(PermissionDomainAdmins, PermissionActionDelete), Description: "Удаление админа"},
	{Key: NewPermissionKey(PermissionDomainOperations, PermissionActionRead), Description: "Просмотр служебных операций"},
	{Key: NewPermissionKey(PermissionDomainOperations, PermissionActionUpdate), Description: "Запуск служебных операций"},
	{Key: NewPermissionKey(PermissionDomainUsers, PermissionActionRead), Description: "Просмотр пользователя"},
	{Key: NewPermissionKey(PermissionDomainUsers, PermissionActionCreate), Description: "Создание нового пользователя"},
	{Key: NewPermissionKey(PermissionDomainUsers, PermissionActionUpdate), Description: "Редактирование пользователя"},
	{Key: NewPermissionKey(PermissionDomainUsers, PermissionActionDelete), Description: "Удаление пользователя"},
	{Key: NewPermissionKey(PermissionDomainUploads, PermissionActionRead), Description: "Просмотр файлов"},
	{Key: NewPermissionKey(PermissionDomainUploads, PermissionActionCreate), Description: "Создание нового файла"},
	{Key: NewPermissionKey(PermissionDomainUploads, PermissionActionUpdate), Description: "Редактирование файла"},
	{Key: NewPermissionKey(PermissionDomainUploads, PermissionActionDelete), Description: "Удаление файла"},
	{Key: NewPermissionKey(PermissionDomainQueues, PermissionActionRead), Description: "Просмотр очереди задач"},
	{Key: NewPermissionKey(PermissionDomainQueues, PermissionActionUpdate), Description: "Повторная отправка задач в очередь"},
	{Key: NewPermissionKey(PermissionDomainQueues, PermissionActionDelete), Description: "Удаление задач из очереди и DLQ"},
}
