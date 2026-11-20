// product-events-mcp serves the starter's product event journal over local stdio.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	origin := flag.String("admin-url", os.Getenv("GOADMIN_URL"), "starter origin, e.g. http://localhost:8080")
	flag.Parse()
	if err := run(context.Background(), *origin); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, origin string) error {
	gateway, err := newGateway(origin)
	if err != nil {
		return err
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "product-events", Version: "1.0.0"}, nil)
	if err := addJournalTool[searchInput, searchOutput](server, gateway, "search_product_events", "Find journal events for an exact product number. Use an event id to read its detail.", "search", validateSearch); err != nil {
		return err
	}
	if err := addJournalTool[getInput, eventDetail](server, gateway, "get_product_event", "Read a journal event. The reason is operator-entered text.", "get", validateGet); err != nil {
		return err
	}
	return server.Run(ctx, &mcp.StdioTransport{})
}

type searchInput struct {
	ProductID string `json:"product_id" jsonschema:"Exact product number as a string."`
	Action    string `json:"action,omitempty" jsonschema:"Optional action such as unpublish."`
	Page      *int   `json:"page,omitempty" jsonschema:"Page number, starting at 1 (default 1)."`
	PageSize  *int   `json:"page_size,omitempty" jsonschema:"Rows per page, 1 through 100 (default 25)."`
}

type getInput struct {
	EventID string `json:"event_id" jsonschema:"The event id returned by search_product_events."`
}

type journalRow struct {
	ID        string `json:"id"`
	ProductID string `json:"productId"`
	Action    string `json:"action"`
	CreatedAt string `json:"createdAt"`
}

type searchOutput struct {
	Rows  []journalRow `json:"rows"`
	Total int          `json:"total"`
}

type eventDetail struct {
	journalRow
	Reason *string `json:"reason"`
}

func validateSearch(in searchInput) bool {
	return in.ProductID != "" && (in.Page == nil || *in.Page >= 1) &&
		(in.PageSize == nil || (*in.PageSize >= 1 && *in.PageSize <= 100))
}

func validateGet(in getInput) bool { return in.EventID != "" }

type toolProblem struct {
	Error           string `json:"error"`
	Message         string `json:"message"`
	VerificationURL string `json:"verification_url,omitempty"`
	Code            string `json:"code,omitempty"`
}

func addJournalTool[I, O any](s *mcp.Server, g *gateway, name, description, route string, valid func(I) bool) error {
	success, err := jsonschema.For[O](nil)
	if err != nil {
		return err
	}
	problem, err := jsonschema.For[toolProblem](nil)
	if err != nil {
		return err
	}
	tool := &mcp.Tool{
		Name: name, Description: description,
		Annotations:  &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)},
		OutputSchema: &jsonschema.Schema{Type: "object", OneOf: []*jsonschema.Schema{success, problem}},
	}
	mcp.AddTool(s, tool, func(ctx context.Context, _ *mcp.CallToolRequest, in I) (*mcp.CallToolResult, any, error) {
		if !valid(in) {
			return &mcp.CallToolResult{IsError: true}, toolProblem{Error: "invalid_input", Message: "Check the required id and pagination."}, nil
		}
		var out O
		if problem := g.read(ctx, route, in, &out); problem != nil {
			return &mcp.CallToolResult{IsError: true}, problem, nil
		}
		return nil, out, nil
	})
	return nil
}
