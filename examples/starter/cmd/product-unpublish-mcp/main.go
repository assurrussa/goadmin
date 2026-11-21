// product-unpublish-mcp lets a local client propose and track a Products action.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type proposal struct {
	RequestID string `json:"request_id" jsonschema:"Generate a UUID before the first call and reuse it for all retries of this exact proposal."`
	ProductID string `json:"product_id" jsonschema:"Positive decimal product ID."`
	Reason    string `json:"reason" jsonschema:"Reason the operator will review and record in the product journal, up to 500 Unicode characters."`
}

type lookup struct {
	RequestID string `json:"request_id" jsonschema:"Original UUID of the proposal."`
}

// The wire response deliberately exposes neither the reason nor credentials.
type outcome struct {
	RequestID       string `json:"request_id,omitempty"`
	Status          string `json:"status,omitempty"`
	ReviewURL       string `json:"review_url,omitempty"`
	EventID         string `json:"event_id,omitempty"`
	Error           string `json:"error,omitempty"`
	Message         string `json:"message,omitempty"`
	VerificationURL string `json:"verification_url,omitempty"`
	Code            string `json:"code,omitempty"`
}

func problem(code, message string) outcome { return outcome{Error: code, Message: message} }

func validKey(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}

func validID(value string) bool {
	id, err := strconv.ParseInt(value, 10, 64)
	return err == nil && id > 0 && strconv.FormatInt(id, 10) == value
}

func response(out outcome) (*mcp.CallToolResult, outcome, error) {
	return &mcp.CallToolResult{IsError: out.Error != ""}, out, nil
}

func main() {
	origin := flag.String("admin-url", os.Getenv("GOADMIN_ADMIN_URL"), "starter admin origin (HTTPS or loopback HTTP)")
	flag.Parse()
	g, err := newClient(*origin)
	if err != nil {
		log.Fatal(err)
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "product-unpublish-mcp", Version: "1"}, &mcp.ServerOptions{
		Instructions: "Generate and retain a canonical UUID for each proposed unpublish. Ask the operator to review the exact reason in the admin browser. If a response is lost, use get_unpublish_request with the same UUID.",
	})
	mcp.AddTool(server, &mcp.Tool{
		Name:        "propose_unpublish",
		Description: "Propose withdrawing a published product. This creates a request for the operator. Only their browser confirmation changes the product.",
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true, DestructiveHint: new(false), OpenWorldHint: new(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in proposal) (*mcp.CallToolResult, outcome, error) {
		reason := strings.TrimSpace(in.Reason)
		if !validKey(in.RequestID) || !validID(in.ProductID) || reason == "" || utf8.RuneCountInString(reason) > 500 {
			return response(problem("invalid_input", "Supply a canonical UUID, positive product ID and a reason of 1–500 characters."))
		}
		return response(g.call(ctx, "propose", in.RequestID, in))
	})
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_unpublish_request",
		Description: "Read the outcome of your proposal using its original UUID, including its journal event number after approval.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in lookup) (*mcp.CallToolResult, outcome, error) {
		if !validKey(in.RequestID) {
			return response(problem("invalid_input", "Supply the original canonical UUID."))
		}
		return response(g.call(ctx, "status", in.RequestID, in))
	})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
