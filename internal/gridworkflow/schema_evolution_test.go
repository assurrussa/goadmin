//nolint:goconst // Literal schema keys keep persisted-document fixtures readable.
package gridworkflow //nolint:testpackage // Exercises internal stored-schema reconciliation.

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchemaEvolutionAddsOnlyNewColumnsPerView(t *testing.T) {
	var original Payload
	require.NoError(t, json.Unmarshal([]byte(`{
  "schemaVersion":1,"columns":["name"],"knownColumns":["id","name"],
  "views":[
    {"id":"old","name":"Old","state":{"columns":["id"],"knownColumns":["id"],
      "sortBy":"id","sortOrder":"asc","limit":25}},
    {"id":"hidden","name":"Hidden","state":{"columns":["name"],"knownColumns":["id","name","secret"],
      "sortBy":"id","sortOrder":"asc","limit":25}}
  ]}`), &original))
	store := &memoryStore{payload: original, version: 4}
	app, _ := testApp(t, store, testConfig(), nil)
	r, err := app.Test(httptest.NewRequestWithContext(t.Context(), "GET", "/queues/table-preferences", nil))
	require.NoError(t, err)
	defer r.Body.Close()
	var got Preferences
	require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
	require.Equal(t, []string{"name", "secret"}, got.Payload.Columns)
	require.Equal(t, []string{"id", "name", "secret"}, got.Payload.Views[0].State.Columns)
	require.Equal(t, []string{"name"}, got.Payload.Views[1].State.Columns)
	require.Equal(t, int64(4), got.Version)
	require.Zero(t, store.saves)
	require.Equal(t, []string{"id"}, store.payload.Views[0].State.Columns)
	require.NoError(t, newSchema(testConfig()).validate(got.Payload))
}

func TestSavingPreferencesCapturesSchemaForLegacyPayload(t *testing.T) {
	store := &memoryStore{}
	app, _ := testApp(t, store, testConfig(), nil)
	body := `{"version":0,"payload":{"schemaVersion":1,"columns":["name"],"views":[
  {"id":"v","name":"Mine","state":{"columns":["id"],"sortBy":"id","sortOrder":"asc","limit":25}}
  ]}}`
	req := httptest.NewRequestWithContext(t.Context(), "PUT", "/queues/table-preferences", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r, err := app.Test(req)
	require.NoError(t, err)
	defer r.Body.Close()
	require.Equal(t, 200, r.StatusCode)
	require.Equal(t, []string{"id", "name", "secret"}, store.payload.KnownColumns)
	view := store.payload.Views[0].State
	require.Equal(t, store.payload.KnownColumns, view.KnownColumns)
	require.Equal(t, []string{"name"}, store.payload.Columns)
	require.Equal(t, []string{"id"}, view.Columns)
}
