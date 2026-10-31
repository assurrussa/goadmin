package auth_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/internal/auth"
)

func TestBrandingCatalogHasSeparateReadAndUpdatePermissions(t *testing.T) {
	catalog := auth.DefaultPermissionDefinitions()
	for _, action := range []auth.PermissionAction{auth.PermissionActionRead, auth.PermissionActionUpdate} {
		definition, err := catalog.Get(auth.PermissionDomainBranding, action)
		require.NoError(t, err)
		require.Equal(t, auth.NewPermissionKey(auth.PermissionDomainBranding, action), definition.Key)
	}
	_, err := catalog.Get(auth.PermissionDomainBranding, auth.PermissionActionDelete)
	require.ErrorIs(t, err, auth.ErrPermissionNotFound)
}
