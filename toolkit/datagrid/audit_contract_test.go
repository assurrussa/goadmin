//nolint:goconst // Independent contract fixtures intentionally repeat wire literals.
package datagrid_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Masterminds/squirrel"
	logger "github.com/assurrussa/gologger"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

// These public contract tests retain pagination and typed-value transport differences
// while protecting the shared field boundary and corrected round-trip behavior.
type auditRow struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type auditRepo func(context.Context, datagrid.Filtered) ([]auditRow, int, error)

func (r auditRepo) GetList(ctx context.Context, f datagrid.Filtered) ([]auditRow, int, error) {
	return r(ctx, f)
}

func auditConfig(repo auditRepo) datagrid.FilterConfig[auditRow] {
	return datagrid.FilterConfig[auditRow]{
		RoutePath: "/audit", Repository: repo, PageSize: 25,
		DefaultSort: "name", DefaultOrder: "asc",
		Columns: []datagrid.Column{
			{Key: "name", Type: "text", Sortable: true, Filterable: true},
			{Key: "count", Type: "number", Filterable: true},
			{Key: "enabled", Type: "boolean", Filterable: true},
			{Key: "day", Type: "date", Filterable: true},
			{Key: "status", Type: "select", Filterable: true, FilterOptions: []map[string]string{{"value": "active", "label": "Active"}}},
			{Key: "private", Type: "text"},
		},
	}
}

func auditRequest(t *testing.T, cfg datagrid.FilterConfig[auditRow], method, path, body string) (int, []byte) {
	t.Helper()
	app := fiber.New()
	datagrid.NewHandler(cfg, logger.Discard()).RegisterRoutes(app, "/audit")
	req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	bs, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, bs
}

func TestAuditPublicGETAndPOSTFilterContracts(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, body string
		limit                    int
		sortBy, sortOrder        string
		fields                   map[string]any
	}{
		{
			name: "GET normalizes and allowlists", method: http.MethodGet,
			path: "/audit/data?page=-1&limit=1000&sortBy=private&sortOrder=SIDEWAYS" +
				"&name=+Alice+&count=7&enabled=true&private=hidden&unknown=x",
			limit: 25, sortBy: "name", sortOrder: "asc", fields: map[string]any{"name": "Alice", "count": 7, "enabled": true},
		},
		{
			name: "POST keeps JSON scalar types and its limit", method: http.MethodPost, path: "/audit/data",
			body: `{"page":1,"limit":1000,"sortBy":"name","sortOrder":"asc",` +
				`"fields":{"count":7,"name":"Alice","enabled":true}}`,
			limit: 1000, sortBy: "name", sortOrder: "asc",
			fields: map[string]any{"count": float64(7), "name": "Alice", "enabled": true},
		},
		{
			name: "empty POST falls back to query", method: http.MethodPost, path: "/audit/data?limit=100&name=Alice",
			limit: 100, sortBy: "name", sortOrder: "asc", fields: map[string]any{"name": "Alice"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			cfg := auditConfig(func(_ context.Context, f datagrid.Filtered) ([]auditRow, int, error) {
				calls++
				require.Equal(t, 1, f.GetPage())
				require.Equal(t, tc.limit, f.GetLimit())
				require.Equal(t, tc.sortBy, f.GetSortBy())
				require.Equal(t, tc.sortOrder, f.GetSortOrder())
				require.Equal(t, tc.fields, f.GetFields())
				return nil, 0, nil
			})
			status, _ := auditRequest(t, cfg, tc.method, tc.path, tc.body)
			require.Equal(t, http.StatusOK, status)
			require.Equal(t, 1, calls)
		})
	}
}

func TestAuditPublicValidationStopsRepositoryButHTMLResetsAllFilters(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, body string
		status, calls            int
	}{
		{"API invalid select", "GET", "/audit/data?status=unknown", "", 400, 0},
		{"API negative page", "POST", "/audit/data", `{"page":-1}`, 400, 0},
		{"API excessive limit", "POST", "/audit/data", `{"limit":1001}`, 400, 0},
		{"HTML invalid select resets", "GET", "/audit?status=unknown&search=Alice&page=3&limit=50&name=Alice", "", 200, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			cfg := auditConfig(func(_ context.Context, f datagrid.Filtered) ([]auditRow, int, error) {
				calls++
				require.Equal(t, 1, f.GetPage())
				require.Equal(t, 25, f.GetLimit())
				require.Empty(t, f.GetSearch())
				require.Empty(t, f.GetFields())
				return nil, 0, nil
			})
			status, _ := auditRequest(t, cfg, tc.method, tc.path, tc.body)
			require.Equal(t, tc.status, status)
			require.Equal(t, tc.calls, calls)
		})
	}
}

func TestAuditPublicFilterMetadataAndDatePaginationRoundTrip(t *testing.T) {
	var seen []datagrid.Filters
	cfg := auditConfig(func(_ context.Context, f datagrid.Filtered) ([]auditRow, int, error) {
		parsed, ok := f.(datagrid.Filters)
		require.True(t, ok)
		seen = append(seen, parsed)
		return []auditRow{{ID: 1}}, 50, nil
	})
	status, bs := auditRequest(t, cfg, "GET", "/audit/data?search=needle&name=Alice&count=7&enabled=true&day=2026-10-02", "")
	require.Equal(t, 200, status)
	var api datagrid.APIResponse[auditRow]
	require.NoError(t, json.Unmarshal(bs, &api))
	require.Equal(t, map[string]string{
		"name": "Alice", "count": "7", "enabled": "true", "day": "2026-10-02", "_search": "needle",
	}, api.Meta.Filters)
	require.Equal(t, "needle", seen[0].Search)
	require.Equal(t, time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), seen[0].Fields["day"])
	require.Equal(t, []int{10, 25, 50, 100}, api.Meta.Pagination.PageSizeOptions)

	// Following the generated URL must apply the same typed filters.
	u, err := url.Parse(api.Meta.Pagination.NextPageURL)
	require.NoError(t, err)
	require.Equal(t, "2026-10-02", u.Query().Get("day"))
	status, _ = auditRequest(t, cfg, "GET", u.String(), "")
	require.Equal(t, 200, status)
	require.Equal(t, seen[0].Fields, seen[1].Fields)
	require.Equal(t, seen[0].Search, seen[1].Search)
	require.Equal(t, 2, seen[1].Page)
}

func TestAuditPublicTenRowPaginationPreservesCustomPageSize(t *testing.T) {
	var limits []int
	cfg := auditConfig(func(_ context.Context, f datagrid.Filtered) ([]auditRow, int, error) {
		limits = append(limits, f.GetLimit())
		return []auditRow{{ID: 1}}, 50, nil
	})
	status, bs := auditRequest(t, cfg, "GET", "/audit/data?limit=10", "")
	require.Equal(t, 200, status)
	var api datagrid.APIResponse[auditRow]
	require.NoError(t, json.Unmarshal(bs, &api))
	require.Contains(t, api.Meta.Pagination.NextPageURL, "limit=10")
	status, _ = auditRequest(t, cfg, "GET", api.Meta.Pagination.NextPageURL, "")
	require.Equal(t, 200, status)
	require.Equal(t, []int{10, 10}, limits)
}

func TestAuditPublicSearchModesAreMetadataOnly(t *testing.T) {
	for _, mode := range []datagrid.SearchMode{datagrid.SearchModeServer, datagrid.SearchModeClient, datagrid.SearchModeNone} {
		t.Run(string(mode), func(t *testing.T) {
			calls := 0
			cfg := auditConfig(func(_ context.Context, f datagrid.Filtered) ([]auditRow, int, error) {
				calls++
				require.Equal(t, "needle", f.GetSearch())
				require.Equal(t, 25, f.GetLimit())
				return nil, 0, nil
			})
			cfg.SearchMode = mode
			status, bs := auditRequest(t, cfg, "GET", "/audit/data?search=needle", "")
			require.Equal(t, 200, status)
			require.Equal(t, 1, calls)
			var api datagrid.APIResponse[auditRow]
			require.NoError(t, json.Unmarshal(bs, &api))
			require.Equal(t, mode, api.Config.Behaviour.SearchMode)
		})
	}
}

func TestAuditPublicRepositoryErrorsAreHiddenForAPIAndDefaultHTML(t *testing.T) {
	cfg := auditConfig(func(context.Context, datagrid.Filtered) ([]auditRow, int, error) {
		return nil, 0, errors.New("private database detail")
	})
	status, bs := auditRequest(t, cfg, "GET", "/audit/data", "")
	require.Equal(t, 500, status)
	require.Equal(t, "Internal Server Error", string(bs))
	status, bs = auditRequest(t, cfg, "GET", "/audit", "")
	require.Equal(t, 500, status)
	require.Equal(t, "Internal Server Error", string(bs))
}

func TestAuditPublicSQLHelpersKeepAllowlistBoundary(t *testing.T) {
	filters := datagrid.Filters{
		Page: 2, Limit: 10, SortBy: "name", SortOrder: "desc",
		Fields: map[string]any{"name": "Alice", "private": "ignored"},
	}
	query := datagrid.SQLBuilderx(squirrel.Select("id").From("users"), filters, map[string]bool{"name": true})
	query = datagrid.SQLWherex(query, filters, map[string]datagrid.FieldMapping{"name": datagrid.NewFieldMapping("display_name")})
	sql, args, err := query.ToSql()
	require.NoError(t, err)
	require.Equal(t, "SELECT id FROM users WHERE display_name = ? ORDER BY name desc LIMIT 10 OFFSET 10", sql)
	require.Equal(t, []any{"Alice"}, args)
	filters.SortBy = "name; DROP TABLE users"
	query = datagrid.SQLSortx(squirrel.Select("id").From("users"), filters, map[string]bool{"name": true})
	sql, _, err = query.ToSql()
	require.NoError(t, err)
	require.Equal(t, "SELECT id FROM users", sql)
}

func TestAuditPublicJSONBKeyOperatorsBindUntrustedValues(t *testing.T) {
	// This is a string-only SQL construction probe; no database is contacted.
	for _, tc := range []struct {
		name string
		op   datagrid.WhereOperator
		val  any
	}{
		{"single key", datagrid.OpJSONBHasKey, "x' OR TRUE --"},
		{"JSON-decoded array", datagrid.OpJSONBHasAnyKey, []any{"x' OR TRUE --"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query := datagrid.SQLWherex(
				squirrel.Select("id").From("users"),
				datagrid.Filters{Fields: map[string]any{"key": tc.val}},
				map[string]datagrid.FieldMapping{"key": datagrid.NewFieldMappingWithOp("metadata", tc.op)},
			)
			sql, args, err := query.ToSql()
			require.NoError(t, err)
			require.NotContains(t, sql, "x' OR TRUE --")
			require.Equal(t, []any{"x' OR TRUE --"}, args)
		})
	}
	query := datagrid.SQLWherex(
		squirrel.Select("id").From("users").PlaceholderFormat(squirrel.Dollar),
		datagrid.Filters{Fields: map[string]any{"key": "safe"}},
		map[string]datagrid.FieldMapping{"key": datagrid.NewFieldMappingWithOp("metadata", datagrid.OpJSONBHasKey)},
	)
	sql, args, err := query.ToSql()
	require.NoError(t, err)
	require.Equal(t, "SELECT id FROM users WHERE metadata ? $1", sql)
	require.Equal(t, []any{"safe"}, args)
}

func TestAuditPublicActionsAndValuesWireShape(t *testing.T) {
	cfg := auditConfig(func(context.Context, datagrid.Filtered) ([]auditRow, int, error) {
		return []auditRow{{ID: 7, Name: "original"}}, 1, nil
	})
	cfg.Actions = []datagrid.Action[auditRow]{
		{Key: "open", Label: "Open"},
		{Key: "denied", CanView: func(context.Context, auditRow) bool { return false }},
	}
	resolve := func(_ context.Context, row auditRow) (map[string]any, error) {
		return map[string]any{"name": "computed", "label": fmt.Sprintf("row %d", row.ID)}, nil
	}
	cfg.RowDecorators = []datagrid.RowDecorator[auditRow]{datagrid.NewRowDecoratorResolve(resolve)}
	status, bs := auditRequest(t, cfg, "GET", "/audit/data", "")
	require.Equal(t, 200, status)
	var api datagrid.APIResponse[auditRow]
	require.NoError(t, json.Unmarshal(bs, &api))
	require.Len(t, api.Data, 1)
	require.Equal(t, "original", api.Data[0].Item.Name)
	require.Equal(t, "computed", api.Data[0].Values["name"])
	require.Equal(t, "row 7", api.Data[0].Values["label"])
	require.Len(t, api.Data[0].Actions, 1)
	require.Equal(t, "open", api.Data[0].Actions[0].Key)
	require.NotContains(t, string(bs), "CanView")
}
