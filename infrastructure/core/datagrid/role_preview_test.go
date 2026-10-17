package datagrid //nolint:testpackage // Exercises visibility metadata without a renderer or DB.

import (
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	logger "github.com/assurrussa/gologger"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRolePreviewFiltersLegacyGridActionsAndCreation(t *testing.T) {
	grid := NewHandler(FilterConfig[int]{RoutePath: "/users", Actions: []Action[int]{{Key: "view"}, {Key: "edit"}, {Key: "delete"}, {Key: "restore"}}}, logger.Discard())
	ctx := integrationroles.WithRolePreview(t.Context(), map[integrationroles.PermissionKey]bool{integrationroles.NewPermissionKey("users", "read"): true})
	actions := grid.availableActions(ctx, 1)
	require.Len(t, actions, 1)
	require.Equal(t, "view", actions[0].Key)
	require.False(t, grid.previewActionAllowed(ctx, "create"))
	require.Len(t, grid.availableActions(t.Context(), 1), 4, "ordinary action callbacks are unchanged")
	require.True(t, grid.previewActionAllowed(integrationroles.WithSuperRolePreview(t.Context()), "create"))
}
