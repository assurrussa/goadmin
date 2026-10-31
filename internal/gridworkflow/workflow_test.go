//nolint:goconst,lll // Repeated contract fixtures remain self-contained for audit.
package gridworkflow //nolint:testpackage // Exercises shared validation and CSV internals.

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	logger "github.com/assurrussa/gologger"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	"github.com/assurrussa/goadmin/infrastructure/core/datagrid"
)

type testRow struct {
	ID     int64  `json:"id"`
	Secret string `json:"secret"`
	Name   string `json:"name"`
}
type memoryStore struct {
	owner     int64
	namespace string
	payload   Payload
	version   int64
	err       error
	saves     int
}

func (s *memoryStore) Load(_ context.Context, owner int64, key string) (Payload, int64, error) {
	s.owner = owner
	s.namespace = key
	return s.payload, s.version, s.err
}

func (s *memoryStore) Save(_ context.Context, owner int64, key string, version int64, p Payload) (int64, error) {
	if s.err != nil {
		return 0, s.err
	}
	if version != s.version {
		return 0, ErrConflict
	}
	s.owner = owner
	s.namespace = key
	s.version++
	s.payload = p
	s.saves++
	return s.version, nil
}

func testConfig() datagrid.FilterConfig[testRow] {
	return datagrid.FilterConfig[testRow]{
		RoutePath: "/queues", DefaultSort: "id", DefaultOrder: "desc", PageSize: 25,
		Columns:    []datagrid.Column{{Key: "id", Label: "ID", Sortable: true}, {Key: "name", Label: "Name", Filterable: true}, {Key: "secret", Label: "Payload"}},
		Repository: datagrid.NewRepositoryAdapter(func(context.Context, datagrid.Filtered) ([]testRow, int, error) { return nil, 0, nil }),
	}
}

func testApp(t *testing.T, store Store, config datagrid.FilterConfig[testRow], selected func(context.Context, []string) ([]testRow, error)) (*fiber.App, *datagrid.Handler[testRow]) {
	t.Helper()
	app := fiber.New()
	grid := datagrid.NewHandler(config, logger.Discard())
	require.NoError(t, Register(app.Group("/queues"), grid, Options[testRow]{Key: "queues", Store: store, CurrentAdmin: func(fiber.Ctx) (int64, error) { return 9007199254740993, nil }, SelectionID: func(_ testRow) string { return cell(json.Number("9007199254740993")) }, LoadSelected: selected}))
	app.Get("/queues/data", grid.HandleData)
	return app, grid
}

func TestPreferencesOwnerValidationConflictAndDatabaseFailure(t *testing.T) {
	store := &memoryStore{}
	app, _ := testApp(t, store, testConfig(), nil)
	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), "GET", "/queues/table-preferences?owner=2&namespace=other", nil))
	require.NoError(t, err)
	var got Preferences
	require.NoError(t, json.NewDecoder(response.Body).Decode(&got))
	require.NoError(t, response.Body.Close())
	require.Equal(t, "9007199254740993", got.Owner)
	require.Equal(t, int64(9007199254740993), store.owner)
	require.Equal(t, "queues", store.namespace)
	require.Equal(t, 0, store.saves)
	require.Empty(t, got.Payload.Views)
	body := `{"version":0,"payload":{"schemaVersion":1,"columns":["name","id"],"views":[]}}`
	put := func(body string) int {
		req := httptest.NewRequestWithContext(t.Context(), "PUT", "/queues/table-preferences", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r, e := app.Test(req)
		require.NoError(t, e)
		require.NoError(t, r.Body.Close())
		return r.StatusCode
	}
	require.Equal(t, 200, put(body))
	require.Equal(t, 409, put(body))
	require.Equal(t, 1, store.saves)
	require.Equal(t, 400, put(strings.Replace(body, `"name","id"`, `"password"`, 1)))
	require.Equal(t, 400, put(strings.Replace(body, `"version":0`, `"version":0,"owner":"2"`, 1)))
	require.Equal(t, 413, put(strings.Repeat(" ", MaxPayloadBytes+1)))
	store.err = errors.New("database down")
	response, err = app.Test(httptest.NewRequestWithContext(t.Context(), "GET", "/queues/table-preferences", nil))
	require.NoError(t, err)
	require.Equal(t, 500, response.StatusCode)
	require.NoError(t, response.Body.Close())
}

func TestReconcileStaleColumnsWithoutWriting(t *testing.T) {
	cfg := testConfig()
	s := newSchema(cfg)
	original := Payload{SchemaVersion: 1, Columns: []string{"gone", "name"}, DefaultViewID: "missing", Views: []View{{ID: "v", Name: "My view", State: State{Columns: []string{"gone"}, SortBy: "gone", SortOrder: "bad", Limit: 999, Filters: map[string]string{"gone": "bad", "name": "Bob"}}}}}
	got := s.reconcile(original)
	require.Equal(t, []string{"name"}, got.Columns)
	require.Equal(t, []string{"id", "name", "secret"}, got.Views[0].State.Columns)
	require.Equal(t, map[string]string{"name": "Bob"}, got.Views[0].State.Filters)
	require.Equal(t, "id", got.Views[0].State.SortBy)
	require.Equal(t, 25, got.Views[0].State.Limit)
	require.Empty(t, got.DefaultViewID)
	require.NoError(t, s.validate(got))
}

func TestExportSelectedUsesDirectLookupAndDecoratedValues(t *testing.T) {
	cfg := testConfig()
	cfg.Repository = datagrid.NewRepositoryAdapter(func(context.Context, datagrid.Filtered) ([]testRow, int, error) {
		t.Fatal("selected export must not scan listing")
		return nil, 0, nil
	})
	cfg.RowDecorators = []datagrid.RowDecorator[testRow]{datagrid.NewRowDecoratorResolve(func(_ context.Context, _ testRow) (map[string]any, error) {
		return map[string]any{"secret": "[masked]"}, nil
	})}
	called := false
	app, grid := testApp(t, &memoryStore{}, cfg, func(_ context.Context, ids []string) ([]testRow, error) {
		called = true
		require.Equal(t, []string{"9007199254740993"}, ids)
		return []testRow{{ID: 9007199254740993, Secret: "secret token", Name: "=malicious,\"cell\"\nnext"}}, nil
	})
	require.Equal(t, "/queues/table-export", grid.GetConfig().Workflow.ExportURL)
	r, err := app.Test(httptest.NewRequestWithContext(t.Context(), "GET", "/queues/table-export?scope=selected&ids=9007199254740993&columns=name,id,secret", nil))
	require.NoError(t, err)
	defer r.Body.Close()
	require.True(t, called)
	require.Equal(t, 200, r.StatusCode)
	require.Equal(t, "no-store", r.Header.Get("Cache-Control"))
	records, err := csv.NewReader(r.Body).ReadAll()
	require.NoError(t, err)
	require.Equal(t, []string{"Name", "ID", "Payload"}, records[0])
	require.Equal(t, []string{"'=malicious,\"cell\"\nnext", "9007199254740993", "[masked]"}, records[1])
}

func TestExportFilteredReusesFiltersAndCapsRows(t *testing.T) {
	cfg := testConfig()
	calls := 0
	cfg.Repository = datagrid.NewRepositoryAdapter(func(_ context.Context, f datagrid.Filtered) ([]testRow, int, error) {
		calls++
		require.Equal(t, calls, f.GetPage())
		require.Equal(t, 100, f.GetLimit())
		require.Equal(t, "Alice", f.GetSearch())
		require.Equal(t, "name", f.GetFields()["name"])
		require.Equal(t, "id", f.GetSortBy())
		rows := make([]testRow, 100)
		for i := range rows {
			rows[i] = testRow{Name: "a"}
		}
		return rows, 10001, nil
	})
	app, _ := testApp(t, &memoryStore{}, cfg, nil)
	r, err := app.Test(httptest.NewRequestWithContext(t.Context(), "GET", "/queues/table-export?search=Alice&name=name&columns=name", nil))
	require.NoError(t, err)
	defer r.Body.Close()
	require.Equal(t, 200, r.StatusCode)
	require.Equal(t, "true", r.Header.Get("X-Goadmin-Export-Truncated"))
	require.Equal(t, 100, calls)
	records, err := csv.NewReader(r.Body).ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 10001)
}

func TestExportRejectsInvalidColumnsIDsAndScopes(t *testing.T) {
	app, _ := testApp(t, &memoryStore{}, testConfig(), nil)
	for _, query := range []string{"columns=secret,secret", "columns=password", "scope=all", "scope=selected&ids=1,1", "scope=selected&ids="} {
		r, err := app.Test(httptest.NewRequestWithContext(t.Context(), "GET", "/queues/table-export?"+query, nil))
		require.NoError(t, err)
		require.Equal(t, 400, r.StatusCode)
		require.NoError(t, r.Body.Close())
	}
	for _, raw := range []string{"", strings.Repeat("1,", 100) + "2", "1,1", " 1", "1\n"} {
		_, err := ParseIDs(raw)
		require.Error(t, err)
	}
}

func TestLegacyGridHasNoWorkflowAndOptInRowsHaveStringIdentity(t *testing.T) {
	cfg := testConfig()
	cfg.Repository = datagrid.NewRepositoryAdapter(func(context.Context, datagrid.Filtered) ([]testRow, int, error) {
		return []testRow{{ID: 9007199254740993}}, 1, nil
	})
	legacy := datagrid.NewHandler(cfg, logger.Discard())
	items, err := legacy.DecorateItems(context.Background(), []testRow{{ID: 1}})
	require.NoError(t, err)
	encoded, err := json.Marshal(items)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "selectionId")
	app, _ := testApp(t, &memoryStore{}, cfg, nil)
	r, err := app.Test(httptest.NewRequestWithContext(t.Context(), "GET", "/queues/data", nil))
	require.NoError(t, err)
	defer r.Body.Close()
	encoded, err = io.ReadAll(r.Body)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"selectionId":"9007199254740993"`)
	require.Contains(t, string(encoded), `"preferencesUrl":"/queues/table-preferences"`)
}

func TestViewValidationRejectsUnsupportedState(t *testing.T) {
	s := newSchema(testConfig())
	base := Payload{SchemaVersion: 1, Columns: []string{"id"}, Views: []View{{ID: "view-1", Name: "Mine", State: State{Search: "", Filters: map[string]string{"name": "Alice"}, SortBy: "id", SortOrder: "desc", Limit: 25, Columns: []string{"id"}}}}}
	require.NoError(t, s.validate(base))
	base.Views[0].State.Filters["owner"] = "other"
	require.Error(t, s.validate(base))
	delete(base.Views[0].State.Filters, "owner")
	base.Views[0].State.Limit = 10000
	require.Error(t, s.validate(base))
	base.Views[0].State.Limit = 25
	base.Views = append(base.Views, base.Views[0])
	require.Error(t, s.validate(base))
}

func TestSavedViewPreservesListingPageSizes(t *testing.T) {
	// The list parser accepts every size in 1..100, including the users/roles
	// default of 20. Saving and loading a view must use that same contract.
	for limit := 1; limit <= 100; limit++ {
		t.Run(strconv.Itoa(limit), func(t *testing.T) {
			cfg := testConfig()
			cfg.PageSize = 20
			s := newSchema(cfg)
			p := Payload{SchemaVersion: 1, Columns: []string{"id"}, Views: []View{{
				ID: "mine", Name: "Mine", State: State{
					SortBy: "id", SortOrder: "desc", Limit: limit, Columns: []string{"id"},
				},
			}}}
			require.NoError(t, s.validate(p))
			require.Equal(t, limit, s.reconcile(p).Views[0].State.Limit)
		})
	}
	for _, limit := range []int{-1, 0, 101} {
		t.Run("invalid_"+strconv.Itoa(limit), func(t *testing.T) {
			cfg := testConfig()
			cfg.PageSize = 20
			s := newSchema(cfg)
			state := State{SortBy: "id", SortOrder: "desc", Limit: limit, Columns: []string{"id"}}
			require.Error(t, s.validateState(state))
			p := Payload{SchemaVersion: 1, Columns: []string{"id"}, Views: []View{{ID: "mine", Name: "Mine", State: state}}}
			require.Equal(t, 20, s.reconcile(p).Views[0].State.Limit)
		})
	}
}

func TestPreferencesSaveDefaultTwentyRowView(t *testing.T) {
	cfg := testConfig()
	cfg.PageSize = 20 // Built-in users and roles start with this page size.
	store := &memoryStore{}
	app, _ := testApp(t, store, cfg, nil)
	body := `{"version":0,"payload":{"schemaVersion":1,"columns":["id"],"views":[
  {"id":"mine","name":"Mine","state":{"columns":["id"],"sortBy":"id","sortOrder":"desc","limit":20}}
  ]}}`
	req := httptest.NewRequestWithContext(t.Context(), "PUT", "/queues/table-preferences", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r, err := app.Test(req)
	require.NoError(t, err)
	require.NoError(t, r.Body.Close())
	require.Equal(t, 200, r.StatusCode)
	require.Equal(t, 1, store.saves)

	r, err = app.Test(httptest.NewRequestWithContext(t.Context(), "GET", "/queues/table-preferences", nil))
	require.NoError(t, err)
	defer r.Body.Close()
	var got Preferences
	require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
	require.Equal(t, 20, got.Payload.Views[0].State.Limit)
}

func TestSelectedExportCallbackErrorsKeepSafeStatus(t *testing.T) {
	for _, tc := range []struct {
		name          string
		callbackError error
		status        int
	}{
		{name: "invalid typed identity", callbackError: fiber.NewError(fiber.StatusBadRequest, "private callback detail"), status: 400},
		{name: "database error", callbackError: errors.New("private database detail"), status: 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, _ := testApp(t, &memoryStore{}, testConfig(), func(context.Context, []string) ([]testRow, error) { return nil, tc.callbackError })
			response, err := app.Test(httptest.NewRequestWithContext(t.Context(), "GET", "/queues/table-export?scope=selected&ids=bad-id", nil))
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, tc.status, response.StatusCode)
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.NotContains(t, string(body), "private")
		})
	}
}

func TestWorkflowAPIErrorsBypassProductionInertiaRedirects(t *testing.T) {
	harness := adminappt.NewAppTest(t)
	globalCalls := 0
	productionErrorHandler := harness.App.HTTPManager().MiddlewareErrorListener()
	app := fiber.New(fiber.Config{ErrorHandler: func(c fiber.Ctx, err error) error {
		globalCalls++
		return productionErrorHandler(c, err)
	}})
	store := &memoryStore{version: 1}
	grid := datagrid.NewHandler(testConfig(), logger.Discard())
	require.NoError(t, Register(app.Group("/queues"), grid, Options[testRow]{
		Key: "queues", Store: store,
		CurrentAdmin: func(fiber.Ctx) (int64, error) { return 42, nil },
		SelectionID:  func(testRow) string { return "1" },
	}))
	check := func(method, target, body string, status int) {
		t.Helper()
		req := httptest.NewRequestWithContext(t.Context(), method, target, strings.NewReader(body))
		// Native fetch does not set Accept: application/json or X-Inertia.
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Referer", "/queues")
		response, err := app.Test(req)
		require.NoError(t, err)
		defer response.Body.Close()
		require.Equal(t, status, response.StatusCode)
		require.Empty(t, response.Header.Get("Location"))
		require.Contains(t, response.Header.Get("Content-Type"), "application/json")
		require.Equal(t, "no-store", response.Header.Get("Cache-Control"))
		payload := map[string]string{}
		require.NoError(t, json.NewDecoder(response.Body).Decode(&payload))
		require.NotEmpty(t, payload["message"])
		require.NotContains(t, payload["message"], "private")
		require.Zero(t, globalCalls)
	}
	check("PUT", "/queues/table-preferences", `{"version":0,"payload":{"schemaVersion":1,"columns":["id"],"views":[]}}`, 409)
	check("PUT", "/queues/table-preferences", `{"version":"broken"}`, 400)
	check("GET", "/queues/table-export?columns=private", "", 400)
	store.err = errors.New("private database details")
	check("GET", "/queues/table-preferences", "", 500)
	app.Post("/bulk", APIHandler(func(fiber.Ctx) error { return fiber.NewError(400, "private callback details") }))
	check("POST", "/bulk", "", 400)
}
