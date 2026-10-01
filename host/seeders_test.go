package host_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	adminhost "github.com/assurrussa/goadmin/host"
)

func TestRolePresetSeedFacade(t *testing.T) {
	preset := adminhost.RolePreset{
		Slug: "cms_editor", Name: "CMS editor", IsSystem: true,
		Permissions: []adminhost.PermissionKey{
			adminhost.NewPermissionKey("cms.entry", "read"),
		},
	}

	option := adminhost.WithRolePresets(preset)

	require.NotNil(t, option)
}
