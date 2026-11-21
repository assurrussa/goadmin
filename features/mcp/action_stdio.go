package mcp

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func addActionTools(server *sdk.Server, g *gateway, d description) {
	a := d.Action
	id := Parameter{
		Name: fieldRequestID, Kind: Text, Required: true,
		Description: "Canonical UUID. Generate it once and reuse it for retries of the same proposal.",
	}
	output := objectSchema()
	addParameter(output, id)
	addParameter(output, Parameter{
		Name: fieldStatus, Kind: Text, Required: true,
		Description: "State of the saved request.",
		Enum:        []string{actionPending, actionSucceeded, actionDeclined, actionExpired, actionConflict},
	})
	addParameter(output, Parameter{Name: fieldReviewURL, Kind: Text, Description: "Open in the approving native browser login."})
	addParameter(output, Parameter{Name: fieldExpiresAt, Kind: Text, Description: "Original deadline of a pending proposal."})
	// Both output kinds are JSON strings. If a field changed kind, publish
	// the broader string schema and validate the specific receipt below.
	for _, version := range slices.Sorted(maps.Keys(a.PreviousFields)) {
		for _, field := range a.PreviousFields[version] {
			addParameter(output, Parameter{Name: field.Key, Kind: Text, Description: field.Description})
		}
	}
	for _, field := range a.Fields {
		kind := field.Kind
		if _, historical := output.Properties[field.Key]; historical {
			kind = Text
		}
		addParameter(output, Parameter{Name: field.Key, Kind: kind, Description: field.Description})
	}
	add := func(name, help, route string, fields []Parameter, readOnly bool) {
		input := objectSchema()
		addParameter(input, id)
		for _, p := range fields {
			addParameter(input, p)
		}
		addAdminTool(server, name, help, input, output, readOnly, func(in map[string]json.RawMessage) bool {
			var rawID string
			if json.Unmarshal(in[fieldRequestID], &rawID) != nil || !canonicalActionID(rawID) {
				return false
			}
			return validActionParameters(fields, in, 1)
		}, func(ctx context.Context, in map[string]json.RawMessage) (any, *toolProblem) {
			return g.actionCall(ctx, route, in, a)
		})
	}
	help := d.Description + " An operator confirms in the admin browser. Save the UUID before proposing."
	add(a.Propose, help, "propose", a.Parameters, false)
	add(a.Status, "Retrieve the original request using its UUID, including after a lost response.", fieldStatus, nil, true)
}

func (g *gateway) actionCall(
	ctx context.Context, route string, in map[string]json.RawMessage, action *actionDescription,
) (any, *toolProblem) {
	token, problem := g.credential(ctx)
	if problem != nil {
		return nil, problem
	}
	var out map[string]json.RawMessage
	status, err := g.post(ctx, route, token, in, &out)
	if err != nil || status >= 500 {
		return nil, unknownActionOutcome()
	}
	switch status {
	case http.StatusOK:
	case http.StatusUnauthorized:
		g.mu.Lock()
		if g.token == token {
			g.token = ""
		}
		g.mu.Unlock()
		return nil, &toolProblem{Error: problemExpired, Message: "Pair again to check the original request."}
	case http.StatusForbidden:
		return nil, &toolProblem{Error: problemDenied, Message: "The operator lacks access to this action."}
	case http.StatusNotFound:
		return nil, &toolProblem{Error: "not_found", Message: "The request or eligible record was not found."}
	case http.StatusConflict:
		return nil, &toolProblem{Error: "conflict", Message: "This UUID belongs to another proposal."}
	case http.StatusUnprocessableEntity, http.StatusBadRequest:
		return nil, &toolProblem{Error: problemInvalidInput, Message: "Check the declared parameters."}
	default:
		return nil, unknownActionOutcome()
	}
	var requested, id, state string
	if json.Unmarshal(in[fieldRequestID], &requested) != nil || json.Unmarshal(out[fieldRequestID], &id) != nil ||
		id != requested || json.Unmarshal(out[fieldStatus], &state) != nil {
		return nil, unknownActionOutcome()
	}
	result := map[string]any{fieldRequestID: id, fieldStatus: state}
	switch state {
	case actionPending:
		var review string
		var expires time.Time
		if json.Unmarshal(out[fieldReviewURL], &review) != nil || review != g.origin.String()+g.path+"/requests/"+id ||
			json.Unmarshal(out[fieldExpiresAt], &expires) != nil || expires.IsZero() {
			return nil, unknownActionOutcome()
		}
		result[fieldReviewURL], result[fieldExpiresAt] = review, expires
	case actionSucceeded:
		if !action.decodeReceipt(out, result) {
			return nil, unknownActionOutcome()
		}
	case actionDeclined, actionConflict, actionExpired:
	default:
		return nil, unknownActionOutcome()
	}
	return result, nil
}

func (a *actionDescription) decodeReceipt(out map[string]json.RawMessage, result map[string]any) bool {
	var version string
	if json.Unmarshal(out[receiptSchemaKey], &version) != nil || version == "" {
		return false
	}
	fields := a.Fields
	if version != a.Version {
		previous, declared := a.PreviousFields[version]
		if !declared {
			return false
		}
		fields = previous
	}
	for _, f := range fields {
		raw, exists := out[f.Key]
		if !exists && f.Nullable {
			continue
		}
		value, ok := f.Kind.parse(raw)
		if !exists || !ok {
			return false
		}
		result[f.Key] = value
	}
	return true
}

func unknownActionOutcome() *toolProblem {
	return &toolProblem{
		Error:   "outcome_unknown",
		Message: "Check the original UUID using the status tool once the server is reachable.",
	}
}
