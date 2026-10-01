package host

import (
	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/seeders/adminseed"
)

type (
	RolesSeedOption  = integrationroles.SeedOption
	RolePreset       = integrationroles.RolePreset
	AdminSeedAccount = adminseed.Account
)

// WithPermissionDefinitions appends host-owned permissions to the built-in roles seed catalog.
func WithPermissionDefinitions(definitions ...PermissionDefinition) RolesSeedOption {
	return integrationroles.WithPermissionDefinitions(definitions...)
}

// WithRolePresets appends host-owned canonical roles to the built-in roles seed.
func WithRolePresets(presets ...RolePreset) RolesSeedOption {
	return integrationroles.WithRolePresets(presets...)
}

// NewRolesSeed exposes the built-in roles seed through the stable host package.
func NewRolesSeed(db outbox.StoragePgsqlClient, opts ...RolesSeedOption) *integrationroles.Seed {
	return integrationroles.NewSeed(db, opts...)
}

// NewAdminSeed exposes the built-in admin seed through the stable host package.
func NewAdminSeed(
	auth *AuthAdapter,
	accounts ...AdminSeedAccount,
) *adminseed.Seed {
	if auth == nil {
		return adminseed.NewSeed(nil, accounts...)
	}

	return adminseed.NewSeed(auth.inner, accounts...)
}
