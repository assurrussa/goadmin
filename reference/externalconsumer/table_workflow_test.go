package externalconsumer_test

import (
	"context"
	"testing"

	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

func TestTableWorkflowUsesSupportedToolkitTypes(t *testing.T) {
	config := datagrid.FilterConfig[int64]{
		Workflow: &datagrid.WorkflowConfig{
			Key: "consumer.records", PreferencesURL: "/records/table-preferences", ExportURL: "/records/table-export",
			BulkActions: []datagrid.WorkflowAction{{
				Key: "archive", Label: "Archive", CanView: func(context.Context) bool { return true },
			}},
		},
		SelectionID: func(int64) string { return "9007199254740993" },
	}
	if config.SelectionID(1) != "9007199254740993" || config.Workflow.Key != "consumer.records" {
		t.Fatal("missing optional workflow contract")
	}
}
