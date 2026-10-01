package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/assurrussa/goauth/postgres"
	"github.com/jackc/pgx/v5/stdlib"

	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
)

const roleSeedName = "rolesseed"

type RolePreset struct {
	Slug        string
	Name        string
	Description string
	IsSystem    bool
	Permissions []PermissionKey
}

type SeedOption func(*Seed)

func WithPermissionDefinitions(definitions ...PermissionDefinition) SeedOption {
	return func(seed *Seed) {
		seed.permissionDefinitions = MergePermissionDefinitions(seed.permissionDefinitions, definitions)
	}
}

func WithRolePresets(presets ...RolePreset) SeedOption {
	return func(seed *Seed) {
		for _, preset := range presets {
			preset.Permissions = append([]PermissionKey(nil), preset.Permissions...)
			seed.rolePresets = append(seed.rolePresets, preset)
		}
	}
}

type Seed struct {
	db                    *sql.DB
	roles                 *RoleService
	permissionDefinitions []PermissionDefinition
	rolePresets           []RolePreset
	configErr             error
}

func NewSeed(database outbox.StoragePgsqlClient, options ...SeedOption) *Seed {
	seed := &Seed{permissionDefinitions: ClonePermissionDefinitions(DefaultPermissionDefinitions().Data)}
	if database == nil || database.DB() == nil || database.DB().Pool() == nil {
		seed.configErr = errors.New("roles seed PostgreSQL pool is required")
		return seed
	}
	seed.db = stdlib.OpenDBFromPool(database.DB().Pool())
	service, err := postgres.NewRBAC(seed.db, nil)
	if err != nil {
		seed.configErr = err
		return seed
	}
	seed.roles = NewRoleService(service)
	for _, option := range options {
		if option != nil {
			option(seed)
		}
	}

	return seed
}

func (*Seed) Name() string { return roleSeedName }

func (s *Seed) Handle(ctx context.Context) error {
	if s == nil || s.configErr != nil {
		if s == nil {
			return errors.New("roles seed is nil")
		}

		return s.configErr
	}
	catalog := NewPermissionCatalog(s.permissionDefinitions)
	inputs := make([]CreatePermissionInput, 0, len(catalog.Data))
	for _, definition := range catalog.Data {
		inputs = append(inputs, CreatePermissionInput(definition))
	}
	if err := s.roles.EnsurePermissions(ctx, inputs); err != nil {
		return fmt.Errorf("ensure role seed permissions: %w", err)
	}
	presets, err := s.seedPresets(catalog)
	if err != nil {
		return err
	}
	for _, preset := range presets {
		if err := s.upsertPreset(ctx, preset); err != nil {
			return fmt.Errorf("seed role %q: %w", preset.Slug, err)
		}
	}

	return nil
}

func (s *Seed) seedPresets(catalog PermissionCatalog) ([]RolePreset, error) {
	presets := []RolePreset{
		{
			Slug: SuperAdminRole, Name: "Супер-администратор",
			Description: "Полный доступ ко всем действиям админ-панели",
			IsSystem:    true, Permissions: catalog.GetPermissions(),
		},
		{
			Slug: "content_admin", Name: "Контент-администратор",
			Description: "Просмотр ролей и разрешений; доступ к CMS назначается отдельной CMS-ролью",
			IsSystem:    true,
			Permissions: []PermissionKey{
				NewPermissionKey(PermissionDomainRoles, PermissionActionRead),
				NewPermissionKey(PermissionDomainPermissions, PermissionActionRead),
			},
		},
	}
	known := make(map[PermissionKey]struct{}, len(catalog.Data))
	for _, definition := range catalog.Data {
		known[definition.Key] = struct{}{}
	}
	seen := map[string]struct{}{SuperAdminRole: {}, "content_admin": {}}
	for _, preset := range s.rolePresets {
		preset.Slug = strings.TrimSpace(preset.Slug)
		preset.Name = strings.TrimSpace(preset.Name)
		if preset.Slug == "" || preset.Name == "" {
			return nil, errors.New("role preset slug and name are required")
		}
		if _, found := seen[preset.Slug]; found {
			return nil, fmt.Errorf("duplicate or reserved role preset slug %q", preset.Slug)
		}
		seen[preset.Slug] = struct{}{}
		for _, permission := range preset.Permissions {
			if _, found := known[permission]; !found {
				return nil, fmt.Errorf("role preset %q uses unknown permission %q", preset.Slug, permission.String())
			}
		}
		presets = append(presets, preset)
	}

	return presets, nil
}

func (s *Seed) upsertPreset(ctx context.Context, preset RolePreset) error {
	roles, err := s.roles.ListRoles(ctx, RoleFilter{Slugs: []string{preset.Slug}, IncludeSys: true})
	if err != nil {
		return err
	}
	if len(roles) == 0 {
		_, err = s.roles.CreateRole(ctx, CreateRoleInput{
			Slug: preset.Slug, Name: preset.Name, Description: &preset.Description,
			IsSystem: preset.IsSystem, Permissions: preset.Permissions,
		})

		return err
	}
	_, err = s.roles.UpdateRole(ctx, UpdateRoleInput{
		ID: roles[0].ID, Slug: preset.Slug, Name: preset.Name,
		Description: &preset.Description, IsSystem: pointer(preset.IsSystem),
		Permissions: pointer(append([]PermissionKey(nil), preset.Permissions...)),
	})

	return err
}

func pointer[T any](value T) *T { return &value }
