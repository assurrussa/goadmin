package main

import (
	"github.com/assurrussa/goadmin/features/mcp"
	"github.com/assurrussa/goadmin/host"
)

const (
	productEventsTitle      = "Журнал действий с товарами"
	productEventsPath       = "/product-events"
	productEventsAgentPath  = "/agent/product-events"
	productEventsPermission = "product_events"
)

func productEventsResource() mcp.Resource {
	actions := productEventActions()
	actionValues := make([]string, 1, len(actions)+1)
	for _, action := range actions {
		actionValues = append(actionValues, action.Value)
	}
	searchFields := []mcp.Field{
		{Key: "productId", Description: eventProductHelp, Kind: mcp.PositiveID},
		{Key: "action", Description: eventActionHelp, Kind: mcp.Text},
		{Key: "createdAt", Description: eventTimeHelp, Kind: mcp.Text},
	}
	detailFields := append([]mcp.Field{}, searchFields...)
	detailFields = append(detailFields, mcp.Field{
		Key: unpublishReason.Key, Description: eventReasonHelp, Kind: mcp.Text, Nullable: true,
	})
	return mcp.Resource{
		Path: productEventsAgentPath, Permission: host.NewPermissionKey(productEventsPermission, "read"),
		Name: "product-events", Description: productEventsHelp,
		RecordIDDescription: "Event ID used to open a journal entry.",
		Consent: mcp.Consent{
			Title:       "Product journal connections",
			Description: "Enter the code from your local MCP client to allow it to read your product events and their unpublish reasons.",
			BackHref:    productEventsPath, BackText: productEventsTitle,
		},
		Search: &mcp.Search{
			ToolName:    "search_product_events",
			Description: "Find journal events for an exact product number. Use an event id to read its detail.",
			Filters: []mcp.Filter{
				{FilterKey: "productId", Parameter: mcp.Parameter{
					Name: "product_id", Description: "Exact positive product number as a decimal string.", Kind: mcp.PositiveID, Required: true,
				}},
				{FilterKey: "action", Parameter: mcp.Parameter{
					Name: "action", Description: "Action such as unpublish. Omit or leave blank to match every action.", Kind: mcp.Text, Enum: actionValues,
				}},
			},
			Fields: searchFields,
		},
		Detail: &mcp.Detail{
			ToolName:    "get_product_event",
			Description: "Read a journal event. The reason is operator-entered text.",
			ID:          mcp.Parameter{Name: "event_id", Description: "The event id returned by search_product_events.", Kind: mcp.PositiveID},
			Fields:      detailFields,
		},
	}
}
