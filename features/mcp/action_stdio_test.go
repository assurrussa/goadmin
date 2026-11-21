package mcp //nolint:testpackage // Exercise the transport boundary for ambiguous write responses.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActionClientUsesOnlyOriginalReceipt(t *testing.T) {
	const id = "00000000-0000-4000-8000-000000000001"
	const archiveField = "archive_ref"
	const sequenceField = "sequence"
	replies := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/agent/archive/status", r.URL.Path)
		assert.Equal(t, "Bearer fixture", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(<-replies))
	}))
	t.Cleanup(server.Close)
	g, err := newGateway(server.URL, "/agent/archive")
	require.NoError(t, err)
	g.token, g.validUntil = "fixture", time.Now().Add(time.Minute)
	in := map[string]json.RawMessage{fieldRequestID: json.RawMessage(`"` + id + `"`)}
	action := &actionDescription{
		Version: "v2", Fields: []Field{{Key: archiveField, Kind: Text}},
		PreviousFields: map[string][]Field{"v1": {{Key: sequenceField, Kind: PositiveID}, {Key: "note", Kind: Text, Nullable: true}}},
	}
	replies <- map[string]any{
		fieldRequestID: id, fieldStatus: actionSucceeded, receiptSchemaKey: "v2",
		archiveField: "doc-09", "private_body": "draft",
	}
	result, problem := g.actionCall(t.Context(), fieldStatus, in, action)
	require.Nil(t, problem)
	require.Equal(t, map[string]any{fieldRequestID: id, fieldStatus: actionSucceeded, archiveField: "doc-09"}, result)
	replies <- map[string]any{fieldRequestID: id, fieldStatus: actionSucceeded, receiptSchemaKey: "v1", sequenceField: "7"}
	result, problem = g.actionCall(t.Context(), fieldStatus, in, action)
	require.Nil(t, problem)
	require.Equal(t, map[string]any{fieldRequestID: id, fieldStatus: actionSucceeded, sequenceField: "7"}, result)
	for _, schema := range []string{"v1", "v2", "future", ""} {
		replies <- map[string]any{fieldRequestID: id, fieldStatus: actionSucceeded, receiptSchemaKey: schema}
		_, problem = g.actionCall(t.Context(), fieldStatus, in, action)
		require.NotNil(t, problem)
		require.Equal(t, "outcome_unknown", problem.Error)
	}
	replies <- map[string]any{fieldRequestID: "00000000-0000-4000-8000-000000000002", fieldStatus: actionSucceeded}
	_, problem = g.actionCall(t.Context(), fieldStatus, in, action)
	require.NotNil(t, problem)
	require.Equal(t, "outcome_unknown", problem.Error)
	replies <- map[string]any{fieldRequestID: id, fieldStatus: actionPending, fieldReviewURL: "https://elsewhere.example/review"}
	_, problem = g.actionCall(t.Context(), fieldStatus, in, action)
	require.NotNil(t, problem)
	require.Equal(t, "outcome_unknown", problem.Error)
}

func TestActionReplaysTheOriginalResultSchema(t *testing.T) {
	oldFields := []Field{{Key: "archive_ref", Description: "Archive reference.", Kind: Text}}
	h := actionHandler{action: Action{
		Version: "v2", ResultFields: []Field{{Key: "event_id", Kind: PositiveID}},
		PreviousResultFields: map[string][]Field{"v1": oldFields},
	}}
	receipt := actionRequest{Version: "v1", State: actionSucceeded, Result: map[string]string{"archive_ref": "document-B"}}
	wire, err := h.wireResult(receipt)
	require.NoError(t, err)
	require.Equal(t, "v1", wire[receiptSchemaKey])
	require.Equal(t, "document-B", wire["archive_ref"])
	require.NotContains(t, wire, "event_id")

	h.action.PreviousResultFields = nil
	_, err = h.wireResult(receipt)
	require.ErrorIs(t, err, errActionUnknown)
	h.action.ResultFields = oldFields
	wire, err = h.wireResult(receipt)
	require.NoError(t, err)
	require.Equal(t, "document-B", wire["archive_ref"])
	require.Equal(t, "v2", wire[receiptSchemaKey])
	// Host fields and transport metadata occupy distinct namespaces even
	// when replaying schemas issued before receipt versioning was added.
	for _, key := range []string{"receiptLink", "receipt_schema"} {
		h.action.PreviousResultFields = map[string][]Field{"v1": {{Key: key, Kind: Text, Description: "Original host value."}}}
		require.NoError(t, validateReceiptFields(h.action.PreviousResultFields["v1"]))
		receipt.Result = map[string]string{key: "stored"}
		wire, err = h.wireResult(receipt)
		require.NoError(t, err)
		require.Equal(t, "stored", wire[key])
		require.Equal(t, "v1", wire[receiptSchemaKey])
	}
}
