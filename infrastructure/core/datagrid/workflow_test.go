package datagrid //nolint:testpackage // Exercises request-specific metadata without a renderer.

import (
	"context"
	"testing"

	logger "github.com/assurrussa/gologger"
	"github.com/stretchr/testify/require"

	integrationroles "github.com/assurrussa/goadmin/internal/auth"
)

func TestWorkflowBulkMetadataIsRequestScopedAndPreviewSafe(t *testing.T) {
	grid := NewHandler(FilterConfig[int]{}, logger.Discard())
	grid.ConfigureWorkflow(&WorkflowConfig{Key: "users", BulkURL: "/users/bulk", BulkActions: []WorkflowAction{
		{Key: "delete", CanView: func(context.Context) bool { return false }},
		{Key: "restore", CanView: func(context.Context) bool { return true }},
	}}, nil)
	ordinary := grid.workflowForRequest(t.Context())
	require.Len(t, ordinary.BulkActions, 1)
	require.Equal(t, "restore", ordinary.BulkActions[0].Key)
	require.Len(t, grid.GetConfig().Workflow.BulkActions, 2, "request filtering must not mutate shared configuration")
	preview := grid.workflowForRequest(integrationroles.WithSuperRolePreview(t.Context()))
	require.Empty(t, preview.BulkActions)
	require.Empty(t, preview.BulkURL)
	require.Len(t, ordinary.BulkActions, 1)
}
