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
	fields := []Field{{Key: archiveField, Kind: Text}}
	replies <- map[string]any{fieldRequestID: id, fieldStatus: actionSucceeded, archiveField: "doc-09", "private_body": "draft"}
	result, problem := g.actionCall(t.Context(), fieldStatus, in, fields)
	require.Nil(t, problem)
	require.Equal(t, map[string]any{fieldRequestID: id, fieldStatus: actionSucceeded, archiveField: "doc-09"}, result)
	replies <- map[string]any{fieldRequestID: "00000000-0000-4000-8000-000000000002", fieldStatus: actionSucceeded}
	_, problem = g.actionCall(t.Context(), fieldStatus, in, fields)
	require.NotNil(t, problem)
	require.Equal(t, "outcome_unknown", problem.Error)
	replies <- map[string]any{fieldRequestID: id, fieldStatus: actionPending, fieldReviewURL: "https://elsewhere.example/review"}
	_, problem = g.actionCall(t.Context(), fieldStatus, in, fields)
	require.NotNil(t, problem)
	require.Equal(t, "outcome_unknown", problem.Error)
}
