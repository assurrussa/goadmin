// product-events-mcp serves the starter's product event journal over local stdio.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"example.com/goadmin-starter/internal/productjournal"
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
	if err := addJournalTool[productjournal.SearchInput, searchOutput](server, gateway, "search_product_events",
		"Find journal events for an exact product number. Use an event id to read its detail.",
		"search", productjournal.SearchInput.Valid); err != nil {
		return err
	}
	if err := addJournalTool[productjournal.GetInput, eventDetail](server, gateway, "get_product_event",
		"Read a journal event. The reason is operator-entered text.", "get", productjournal.GetInput.Valid); err != nil {
		return err
	}
	return server.Run(ctx, &mcp.StdioTransport{})
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
