package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// RunStdio serves the configured resource over the process's stdin/stdout.
// The admin origin supplies tool metadata. Device secrets and grants remain
// in local process memory, and credentials are never included in MCP results.
func RunStdio(ctx context.Context, origin, resourcePath string) error {
	gateway, err := newGateway(origin, resourcePath)
	if err != nil {
		return err
	}
	var d description
	status, err := gateway.post(ctx, "describe", "", struct{}{}, &d)
	if err != nil || status != http.StatusOK {
		return errors.New("mcp: could not read the configured resource description")
	}
	if err := d.validate(); err != nil {
		return errors.New("mcp: incompatible resource description")
	}
	server := sdk.NewServer(&sdk.Implementation{Name: d.Name, Version: "1.0.0"}, &sdk.ServerOptions{Instructions: d.Description})
	if d.Action != nil {
		addActionTools(server, gateway, d)
	}
	if s := d.Search; s != nil {
		addReadTool(server, s.ToolName, s.Description, searchInput(*s),
			searchSchema(d.ID, s.Fields),
			func(in map[string]json.RawMessage) bool {
				_, ok := s.filters(in)
				return ok
			}, func(ctx context.Context, in map[string]json.RawMessage) (any, *toolProblem) {
				return readSearch(ctx, gateway, in, s.Fields)
			})
	}
	if detail := d.Detail; detail != nil {
		in := objectSchema()
		addParameter(in, detail.ID)
		addReadTool(server, detail.ToolName, detail.Description, in, recordSchema(d.ID, detail.Fields),
			func(in map[string]json.RawMessage) bool {
				_, ok := detail.recordID(in)
				return ok
			}, func(ctx context.Context, in map[string]json.RawMessage) (any, *toolProblem) {
				id, _ := detail.recordID(in)
				return readDetail(ctx, gateway, in, id, detail.Fields)
			})
	}
	return server.Run(ctx, &sdk.StdioTransport{})
}

type toolProblem struct {
	Error           string `json:"error"`
	Message         string `json:"message"`
	VerificationURL string `json:"verification_url,omitempty"` //nolint:tagliatelle // Existing MCP response.
	Code            string `json:"code,omitempty"`
}

func addReadTool(
	server *sdk.Server, name, description string,
	input, output *jsonschema.Schema, valid func(map[string]json.RawMessage) bool,
	read func(context.Context, map[string]json.RawMessage) (any, *toolProblem),
) {
	addAdminTool(server, name, description, input, output, true, valid, read)
}

func addAdminTool(
	server *sdk.Server, name, description string,
	input, output *jsonschema.Schema, readOnly bool, valid func(map[string]json.RawMessage) bool,
	call func(context.Context, map[string]json.RawMessage) (any, *toolProblem),
) {
	problemSchema := objectSchema()
	for _, parameter := range []Parameter{
		{Name: fieldError, Description: "Machine-readable problem code.", Kind: Text, Required: true},
		{Name: "message", Description: "Operator-facing explanation.", Kind: Text, Required: true},
		{Name: fieldVerificationURL, Description: "Admin page for the operator.", Kind: Text},
		{Name: fieldCode, Description: "Pairing code shown to the operator.", Kind: Text},
	} {
		addParameter(problemSchema, parameter)
	}
	server.AddTool(&sdk.Tool{
		Name: name, Description: description, InputSchema: input,
		Annotations:  &sdk.ToolAnnotations{ReadOnlyHint: readOnly, OpenWorldHint: new(false)},
		OutputSchema: &jsonschema.Schema{Type: "object", AnyOf: []*jsonschema.Schema{output, problemSchema}},
	}, func(ctx context.Context, request *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		raw := request.Params.Arguments
		if len(raw) == 0 {
			raw = json.RawMessage("{}")
		}
		var in map[string]json.RawMessage
		if len(raw) > maxRequestSize || !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) ||
			json.Unmarshal(raw, &in) != nil || !valid(in) {
			return result(nil, &toolProblem{Error: problemInvalidInput, Message: "Check the declared tool parameters."})
		}
		value, problem := call(ctx, in)
		return result(value, problem)
	})
}

func result(value any, problem *toolProblem) (*sdk.CallToolResult, error) {
	if problem != nil {
		value = problem
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("mcp: could not encode the tool result")
	}
	return &sdk.CallToolResult{
		IsError: problem != nil, StructuredContent: json.RawMessage(encoded),
		Content: []sdk.Content{&sdk.TextContent{Text: string(encoded)}},
	}, nil
}

type wireRecord map[string]any

func (w *wireRecord) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return decoder.Decode((*map[string]any)(w))
}

func (w wireRecord) project(fields []Field) (map[string]any, bool) {
	id, _ := w["id"].(string)
	return project(Record{ID: id, Data: w}, fields)
}

type searchOutput struct {
	Rows  []map[string]any `json:"rows"`
	Total int              `json:"total"`
}

func readSearch(ctx context.Context, g *gateway, input map[string]json.RawMessage, fields []Field) (any, *toolProblem) {
	var wire struct {
		Rows  []wireRecord `json:"rows"`
		Total *int         `json:"total"`
	}
	if p := g.read(ctx, "search", input, &wire); p != nil {
		return nil, p
	}
	size, _ := paginationValue(input["page_size"], defaultPageSize)
	if wire.Rows == nil || wire.Total == nil || len(wire.Rows) > size || *wire.Total < len(wire.Rows) {
		return nil, gatewayUnavailable()
	}
	out := searchOutput{Rows: make([]map[string]any, 0, len(wire.Rows)), Total: *wire.Total}
	for _, row := range wire.Rows {
		item, ok := row.project(fields)
		if !ok {
			return nil, gatewayUnavailable()
		}
		out.Rows = append(out.Rows, item)
	}
	return out, nil
}

func readDetail(
	ctx context.Context, g *gateway, input map[string]json.RawMessage, id string, fields []Field,
) (any, *toolProblem) {
	var wire wireRecord
	if p := g.read(ctx, "get", input, &wire); p != nil {
		return nil, p
	}
	out, ok := wire.project(fields)
	if !ok || out["id"] != id {
		return nil, gatewayUnavailable()
	}
	return out, nil
}

func searchInput(s Search) *jsonschema.Schema {
	in := objectSchema()
	if s.Query != nil {
		addParameter(in, *s.Query)
	}
	for _, filter := range s.Filters {
		addParameter(in, filter.Parameter)
	}
	in.Properties["page"] = &jsonschema.Schema{
		Type: string(Integer), Description: "Page number, starting at 1 (default 1).", Minimum: new(float64(1)),
	}
	in.Properties["page_size"] = &jsonschema.Schema{
		Type: string(Integer), Description: "Rows per page, 1 through 100 (default 25).",
		Minimum: new(float64(1)), Maximum: new(float64(maxPageSize)),
	}
	return in
}

func searchSchema(id string, fields []Field) *jsonschema.Schema {
	list := objectSchema()
	list.Required = []string{"rows", "total"}
	list.Properties["rows"] = &jsonschema.Schema{Type: "array", Items: recordSchema(id, fields), MaxItems: new(maxPageSize)}
	list.Properties["total"] = &jsonschema.Schema{Type: string(Integer), Minimum: new(float64(0))}
	return list
}

func recordSchema(id string, fields []Field) *jsonschema.Schema {
	object := objectSchema()
	addParameter(object, Parameter{Name: "id", Description: id, Kind: Text, Required: true})
	for _, field := range fields {
		schema := scalarSchema(field.Kind, field.Description)
		if field.Nullable {
			schema.Types = []string{schema.Type, "null"}
			schema.Type = ""
		}
		object.Properties[field.Key] = schema
		object.Required = append(object.Required, field.Key)
	}
	return object
}

func addParameter(object *jsonschema.Schema, p Parameter) {
	schema := scalarSchema(p.Kind, p.Description)
	for _, option := range p.Enum {
		schema.Enum = append(schema.Enum, option)
	}
	object.Properties[p.Name] = schema
	if p.Required {
		object.Required = append(object.Required, p.Name)
	}
}

func objectSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object", Properties: map[string]*jsonschema.Schema{},
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
}

func scalarSchema(kind Kind, description string) *jsonschema.Schema {
	if kind == PositiveID {
		return &jsonschema.Schema{Type: "string", Pattern: "^[1-9][0-9]{0,18}$", Description: description}
	}
	return &jsonschema.Schema{Type: string(kind), Description: description}
}
