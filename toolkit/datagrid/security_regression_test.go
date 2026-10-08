//nolint:goconst // Independent SQL and wire fixtures intentionally repeat literals.
package datagrid_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

func TestPublicJSONBOperatorsBindEveryOperand(t *testing.T) {
	const payload = "x' OR TRUE -- \\ ?"
	const field = "profile." + payload
	for _, tc := range []struct {
		name, question, dollar string
		op                     datagrid.WhereOperator
		value                  any
		args                   []any
	}{
		{"contains", "@> ?", "@> $2", datagrid.OpJSONBContains, payload, []any{payload}},
		{"contained by", "<@ ?", "<@ $2", datagrid.OpJSONBContainedBy, payload, []any{payload}},
		{"key", "?? ?", "? $2", datagrid.OpJSONBHasKey, payload, []any{payload}},
		{
			"any key", "??| ARRAY[?,?]::text[]", "?| ARRAY[$2,$3]::text[]", datagrid.OpJSONBHasAnyKey,
			[]string{payload, "safe"},
			[]any{payload, "safe"},
		},
		{
			"all keys", "??& ARRAY[?,?]::text[]", "?& ARRAY[$2,$3]::text[]", datagrid.OpJSONBHasAllKeys,
			[]any{payload, "safe"},
			[]any{payload, "safe"},
		},
		{
			"path", "#> ?::text[] = ?", "#> $2::text[] = $3", datagrid.OpJSONBExtractPath,
			payload,
			[]any{"{profile," + payload + "}", payload},
		},
		{
			"path text", "#>> ?::text[] = ?", "#>> $2::text[] = $3", datagrid.OpJSONBExtractPathText,
			payload,
			[]any{"{profile," + payload + "}", payload},
		},
		{
			"field", "-> ?::text = ?", "-> $2::text = $3", datagrid.OpJSONBExtractField,
			payload,
			[]any{payload, payload},
		},
		{
			"field text", "->> ?::text = ?", "->> $2::text = $3", datagrid.OpJSONBExtractFieldText,
			payload,
			[]any{payload, payload},
		},
		{"path exists", "@?? ?", "@? $2", datagrid.OpJSONBPathExists, payload, []any{payload}},
		{"path match", "@@ ?", "@@ $2", datagrid.OpJSONBPathMatch, payload, []any{payload}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query := datagrid.SQLWherex(
				squirrel.Select("id").From("users").Where("tenant_id = ?", 42),
				datagrid.Filters{Fields: map[string]any{field: tc.value, "unknown": payload}},
				map[string]datagrid.FieldMapping{field: datagrid.NewFieldMappingWithOp("metadata", tc.op)},
			)
			// Question format is Squirrel's intermediate SQL: ?? has not yet been unescaped.
			sql, args, err := query.ToSql()
			require.NoError(t, err)
			require.Equal(t, "SELECT id FROM users WHERE tenant_id = ? AND metadata "+tc.question, sql)
			require.NotContains(t, sql, payload)
			wantArgs := append([]any{42}, tc.args...)
			require.Equal(t, wantArgs, args)

			// Dollar format is the PostgreSQL statement and must retain each literal operator.
			sql, args, err = query.PlaceholderFormat(squirrel.Dollar).ToSql()
			require.NoError(t, err)
			require.Equal(t, "SELECT id FROM users WHERE tenant_id = $1 AND metadata "+tc.dollar, sql)
			require.NotContains(t, sql, payload)
			require.Equal(t, wantArgs, args)
		})
	}
}

func TestPublicJSONBArrayBindingsKeepValuesOutOfSQL(t *testing.T) {
	const payload = "x' OR TRUE -- \\ ? , { } \"\nключ"
	for _, op := range []datagrid.WhereOperator{datagrid.OpJSONBHasAnyKey, datagrid.OpJSONBHasAllKeys} {
		for _, tc := range []struct {
			name, expression string
			value            any
			args             []any
		}{
			{"strings", "ARRAY[$1,$2]::text[]", []string{payload, ""}, []any{payload, ""}},
			{"JSON array", "ARRAY[$1,$2,$3]::text[]", []any{payload, float64(7), false}, []any{payload, "7", "false"}},
			{"integers", "ARRAY[$1,$2]::text[]", []int{-1, 0}, []any{"-1", "0"}},
			{"empty strings", "ARRAY[]::text[]", []string{}, nil},
			{"nil strings", "ARRAY[]::text[]", []string(nil), nil},
			{"empty JSON array", "ARRAY[]::text[]", []any{}, nil},
			{"nil JSON array", "ARRAY[]::text[]", []any(nil), nil},
			{"empty integers", "ARRAY[]::text[]", []int{}, nil},
			{"nil integers", "ARRAY[]::text[]", []int(nil), nil},
			{"array literal", "$1::text[]", "{email,phone}", []any{"{email,phone}"}},
			{"scalar payload", "$1::text[]", payload, []any{payload}},
			{"null", "$1::text[]", nil, []any{nil}},
		} {
			t.Run(string(op)+"/"+tc.name, func(t *testing.T) {
				query := datagrid.SQLWherex(
					squirrel.Select("id").From("users").PlaceholderFormat(squirrel.Dollar),
					datagrid.Filters{Fields: map[string]any{"key": tc.value}},
					map[string]datagrid.FieldMapping{"key": datagrid.NewFieldMappingWithOp("metadata", op)},
				)
				sql, args, err := query.ToSql()
				require.NoError(t, err)
				require.Equal(t, "SELECT id FROM users WHERE metadata "+string(op)+" "+tc.expression, sql)
				require.NotContains(t, sql, payload)
				require.Equal(t, tc.args, args)
			})
		}
	}
}

func TestPublicPaginationPreservesTypedFiltersAcrossEveryLink(t *testing.T) {
	var seen datagrid.Filters
	cfg := auditConfig(func(_ context.Context, f datagrid.Filtered) ([]auditRow, int, error) {
		var ok bool
		seen, ok = f.(datagrid.Filters)
		require.True(t, ok)
		return []auditRow{{ID: 1}}, 70, nil
	})
	path := "/audit/data?page=3&limit=10&search=hello%26world&name=A%2BB&count=0&enabled=false&day=2026-10-02"
	status, bs := auditRequest(t, cfg, http.MethodGet, path, "")
	require.Equal(t, http.StatusOK, status)
	var api datagrid.APIResponse[auditRow]
	require.NoError(t, json.Unmarshal(bs, &api))
	require.Equal(t, map[string]string{
		"_search": "hello&world", "name": "A+B", "count": "0", "enabled": "false", "day": "2026-10-02",
	}, api.Meta.Filters)
	initial := seen
	pagination := api.Meta.Pagination
	urls := []string{pagination.FirstPageURL, pagination.LastPageURL, pagination.NextPageURL, pagination.PrevPageURL}
	for _, link := range pagination.Links {
		if link.URL != "" {
			urls = append(urls, link.URL)
		}
	}
	for _, link := range urls {
		parsed, err := url.Parse(link)
		require.NoError(t, err)
		require.Equal(t, "10", parsed.Query().Get("limit"))
		require.Equal(t, "2026-10-02", parsed.Query().Get("day"))
		status, _ = auditRequest(t, cfg, http.MethodGet, link, "")
		require.Equal(t, http.StatusOK, status)
		require.Equal(t, initial.Fields, seen.Fields)
		require.Equal(t, initial.Search, seen.Search)
		require.Equal(t, initial.Limit, seen.Limit)
		require.Equal(t, initial.SortBy, seen.SortBy)
		require.Equal(t, initial.SortOrder, seen.SortOrder)
	}
}

func TestPublicFiltersSerializeEffectiveLimitAndReservedControls(t *testing.T) {
	f := datagrid.Filters{
		Page: 3, Limit: 10, Search: "needle", SortBy: "name", SortOrder: "asc",
		Fields: map[string]any{
			"page": "99", "limit": "999", "search": "hijack", "sortBy": "hidden", "sortOrder": "desc", "_search": "hijack",
			"name": "Alice", "day": time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), "zero": 0, "disabled": false,
		},
	}
	cfg := datagrid.FilterConfig[any]{PageSize: 25}
	params := f.ToQueryParams(cfg)
	require.Equal(t, "10", params.Get("limit"))
	require.Equal(t, "3", params.Get("page"))
	require.Equal(t, "needle", params.Get("search"))
	require.Equal(t, "name", params.Get("sortBy"))
	require.Equal(t, "asc", params.Get("sortOrder"))
	require.False(t, params.Has("_search"))
	require.Equal(t, "2026-10-02", params.Get("day"))
	require.Equal(t, "0", params.Get("zero"))
	require.Equal(t, "false", params.Get("disabled"))
	u, err := url.Parse(f.BuildURL("/audit/data", cfg))
	require.NoError(t, err)
	require.Equal(t, params, u.Query())
	first, last, next, prev := datagrid.BuildPaginationURLs("/audit/data", 3, 7, 10, f)
	for _, link := range []string{first, last, next, prev} {
		u, err = url.Parse(link)
		require.NoError(t, err)
		query := u.Query()
		require.NotEqual(t, "99", query.Get("page"))
		query.Del("page")
		want := f.ToQueryParams(cfg)
		want.Del("page")
		require.Equal(t, want, query)
	}
	// A zero config size means the existing effective default is ten.
	require.False(t, f.ToQueryParams(datagrid.FilterConfig[any]{}).Has("limit"))
	require.Empty(t, (datagrid.Filters{}).ToQueryParams(datagrid.FilterConfig[any]{}))
}

func TestPublicPOSTFilterMetadataPreservesScalarsAndSearch(t *testing.T) {
	cfg := auditConfig(func(_ context.Context, f datagrid.Filtered) ([]auditRow, int, error) {
		require.Equal(t, "needle", f.GetSearch())
		require.NotContains(t, f.GetFields(), "_search")
		require.IsType(t, float64(0), f.GetFields()["count"])
		require.Zero(t, f.GetFields()["count"])
		require.Equal(t, false, f.GetFields()["enabled"])
		return nil, 0, nil
	})
	status, bs := auditRequest(t, cfg, http.MethodPost, "/audit/data",
		`{"search":"needle","fields":{"name":"","count":0,"enabled":false,"day":"2026-10-02"}}`)
	require.Equal(t, http.StatusOK, status)
	var api datagrid.APIResponse[auditRow]
	require.NoError(t, json.Unmarshal(bs, &api))
	require.Equal(t, map[string]string{
		"_search": "needle", "name": "", "count": "0", "enabled": "false", "day": "2026-10-02",
	}, api.Meta.Filters)
}

func TestPublicErrorCallbackPreservesOriginalErrorAndStatusOwnership(t *testing.T) {
	privateErr := errors.New("private repository detail")
	cfg := auditConfig(func(context.Context, datagrid.Filtered) ([]auditRow, int, error) {
		return nil, 0, privateErr
	})
	cfg.Title, cfg.Entity = "Audit", "audit"
	var calls int
	var callback datagrid.ErrorTemplateComponent = func(c fiber.Ctx, data datagrid.ErrorData, err error) error {
		calls++
		require.ErrorIs(t, err, privateErr)
		require.Equal(t, http.StatusInternalServerError, data.Status)
		require.Equal(t, "Audit", data.Title)
		require.Equal(t, "audit", data.Entity)
		require.NotContains(t, data.Message, privateErr.Error())
		return c.Status(http.StatusServiceUnavailable).SendString("Custom error")
	}
	cfg.ErrorComponent = callback
	status, bs := auditRequest(t, cfg, http.MethodGet, "/audit", "")
	require.Equal(t, http.StatusServiceUnavailable, status)
	require.Equal(t, "Custom error", string(bs))
	require.Equal(t, 1, calls)
}

func TestPublicDefaultErrorsRedactDecoratorFailures(t *testing.T) {
	privateErr := errors.New("private decorator detail")
	for _, stage := range []string{"prepare", "resolve"} {
		for _, path := range []string{"/audit", "/audit/data"} {
			t.Run(stage+path, func(t *testing.T) {
				cfg := auditConfig(func(context.Context, datagrid.Filtered) ([]auditRow, int, error) {
					return []auditRow{{ID: 1}}, 1, nil
				})
				prepare := func(context.Context, []auditRow) error { return nil }
				resolve := func(context.Context, auditRow) (map[string]any, error) { return nil, privateErr }
				if stage == "prepare" {
					prepare = func(context.Context, []auditRow) error { return privateErr }
				}
				cfg.RowDecorators = []datagrid.RowDecorator[auditRow]{datagrid.NewRowDecorator(prepare, resolve)}
				status, bs := auditRequest(t, cfg, http.MethodGet, path, "")
				require.Equal(t, http.StatusInternalServerError, status)
				require.Equal(t, "Internal Server Error", string(bs))
			})
		}
	}
}

func TestPublicPageMetadataRetainsTypedFilterValues(t *testing.T) {
	cfg := auditConfig(func(context.Context, datagrid.Filtered) ([]auditRow, int, error) {
		return nil, 0, nil
	})
	var calls int
	cfg.PageComponent = func(c fiber.Ctx, data datagrid.Response[auditRow]) error {
		calls++
		require.Equal(t, map[string]string{
			"name": "Alice", "count": "0", "enabled": "false", "day": "2026-10-02", "_search": "needle",
		}, data.FilterValues)
		require.Equal(t, data.FilterValues, data.ToAPIResponse().Meta.Filters)
		return c.SendStatus(http.StatusOK)
	}
	status, _ := auditRequest(t, cfg, http.MethodGet,
		"/audit?search=needle&name=Alice&count=0&enabled=false&day=2026-10-02", "")
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, 1, calls)
}
