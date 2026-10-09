//nolint:goconst // Contract fixtures intentionally repeat wire values.
package datagrid_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	logger "github.com/assurrussa/gologger"
	gojson "github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

func filterBodyRequest(t *testing.T, app *fiber.App, body, contentType string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		"/audit/data?page=9&limit=100&name=Query", strings.NewReader(body))
	if contentType != "" {
		req.Header.Set(fiber.HeaderContentType, contentType)
	}
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	bs, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, bs
}

func TestPublicPOSTMalformedBodiesStopBeforeRepository(t *testing.T) {
	decoders := map[string]func([]byte, any) error{
		"standard": json.Unmarshal,
		"runtime":  gojson.Unmarshal,
	}
	for decoderName, decoder := range decoders {
		t.Run(decoderName, func(t *testing.T) {
			for _, body := range []string{
				"{",
				"invalid json",
				`{"page":2,"fields":{"name":"secret"}`,
				`{"page":"invalid"}`,
				`{"fields":[]}`,
				`{"fields":{"name":"secret"}} trailing`,
				`{"fields":{"name":"secret"}} {}`,
				`{"fields":{"name":"secret"}} null`,
				"[]",
				" \n\t",
			} {
				t.Run(body, func(t *testing.T) {
					repositoryCalls, errorCalls := 0, 0
					cfg := auditConfig(func(context.Context, datagrid.Filtered) ([]auditRow, int, error) {
						repositoryCalls++
						return nil, 0, nil
					})
					app := fiber.New(fiber.Config{
						JSONDecoder: decoder,
						ErrorHandler: func(c fiber.Ctx, err error) error {
							errorCalls++
							var filterErr *fiber.Error
							require.ErrorAs(t, err, &filterErr)
							require.Equal(t, http.StatusBadRequest, filterErr.Code)
							return c.Status(filterErr.Code).JSON(fiber.Map{"message": filterErr.Message})
						},
					})
					datagrid.NewHandler(cfg, logger.Discard()).RegisterRoutes(app, "/audit")
					status, bs := filterBodyRequest(t, app, body, fiber.MIMEApplicationJSON)
					require.Equal(t, http.StatusBadRequest, status)
					require.JSONEq(t, `{"message":"Invalid filter request body"}`, string(bs))
					require.Zero(t, repositoryCalls)
					require.Equal(t, 1, errorCalls)
				})
			}
		})
	}
}

type filterTestBinder struct{}

func (filterTestBinder) Name() string { return "filter-test" }

func (filterTestBinder) MIMETypes() []string { return []string{"application/x-filter-test"} }

func (filterTestBinder) Parse(c fiber.Ctx, out any) error {
	if string(c.Body()) != "valid" {
		return errors.New("private binder detail")
	}
	filters, ok := out.(*datagrid.Filters)
	if !ok {
		return errors.New("unexpected filter target")
	}
	*filters = datagrid.Filters{Page: 2, Fields: map[string]any{"name": "Custom"}}
	return nil
}

func TestPublicPOSTBodyCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name, body, contentType string
		status, page, limit     int
		fields                  map[string]any
	}{
		{"empty JSON uses query", "", fiber.MIMEApplicationJSON, 200, 9, 100, map[string]any{"name": "Query"}},
		{"empty untyped uses query", "", "", 200, 9, 100, map[string]any{"name": "Query"}},
		{"JSON object owns request", "{}", fiber.MIMEApplicationJSON, 200, 1, 25, nil},
		{"JSON surrounding whitespace", " \n{}\t", fiber.MIMEApplicationJSON, 200, 1, 25, nil},
		{"empty form keeps binder defaults", "", fiber.MIMEApplicationForm, 200, 1, 25, nil},
		{"JSON null retains defaults", "null", fiber.MIMEApplicationJSON, 200, 1, 25, nil},
		{
			"JSON charset", `{"fields":{"name":"Body"}}`, "application/json; charset=utf-8",
			200, 1, 25,
			map[string]any{"name": "Body"},
		},
		{"form binder", "Page=2&Limit=100&SortBy=name&SortOrder=asc", fiber.MIMEApplicationForm, 200, 2, 100, nil},
		{"XML binder", "<Filters><Page>2</Page><Limit>100</Limit></Filters>", fiber.MIMEApplicationXML, 200, 2, 100, nil},
		{"custom binder", "valid", "application/x-filter-test", 200, 2, 25, map[string]any{"name": "Custom"}},
		{"invalid form", "Page=invalid", fiber.MIMEApplicationForm, 400, 0, 0, nil},
		{"invalid XML", "<Filters><Page>", fiber.MIMEApplicationXML, 400, 0, 0, nil},
		{"invalid custom body", "invalid", "application/x-filter-test", 400, 0, 0, nil},
		{"unsupported nonempty body", "name=Body", "text/plain", 400, 0, 0, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			cfg := auditConfig(func(_ context.Context, f datagrid.Filtered) ([]auditRow, int, error) {
				calls++
				require.Equal(t, tc.page, f.GetPage())
				require.Equal(t, tc.limit, f.GetLimit())
				require.Equal(t, "name", f.GetSortBy())
				require.Equal(t, "asc", f.GetSortOrder())
				require.Equal(t, tc.fields, f.GetFields())
				return nil, 0, nil
			})
			app := fiber.New()
			app.RegisterCustomBinder(filterTestBinder{})
			datagrid.NewHandler(cfg, logger.Discard()).RegisterRoutes(app, "/audit")
			status, bs := filterBodyRequest(t, app, tc.body, tc.contentType)
			require.Equal(t, tc.status, status, string(bs))
			if tc.status == http.StatusOK {
				require.Equal(t, 1, calls)
			} else {
				require.Zero(t, calls)
				require.Equal(t, "Invalid filter request body", string(bs))
			}
		})
	}
}

func TestPublicPOSTRejectsUnconfiguredFieldsAndSorting(t *testing.T) {
	for _, body := range []string{
		`{"fields":{"unknown":"value"}}`,
		`{"fields":{"private":"value"}}`,
		`{"fields":{"unknown":null}}`,
		`{"fields":{"page":99}}`,
		`{"fields":{"_search":"hidden"}}`,
		`{"sortBy":"private"}`,
		`{"sortBy":"unknown"}`,
		`{"sortBy":"name; DROP TABLE users"}`,
		`{"sortOrder":"SIDEWAYS"}`,
		`{"fields":{"status":"unknown"}}`,
		`{"fields":{"status":false}}`,
		`{"fields":{"status":7}}`,
		`{"fields":{"status":["active"]}}`,
		`{"fields":{"status":{"value":"active"}}}`,
	} {
		t.Run(body, func(t *testing.T) {
			calls := 0
			cfg := auditConfig(func(context.Context, datagrid.Filtered) ([]auditRow, int, error) {
				calls++
				return nil, 0, nil
			})
			status, _ := auditRequest(t, cfg, http.MethodPost, "/audit/data?name=Query", body)
			require.Equal(t, http.StatusBadRequest, status)
			require.Zero(t, calls)
		})
	}
}

func TestPublicGETKeepsFieldAndSortFallbacks(t *testing.T) {
	calls := 0
	cfg := auditConfig(func(_ context.Context, f datagrid.Filtered) ([]auditRow, int, error) {
		calls++
		require.Equal(t, map[string]any{"name": "Alice"}, f.GetFields())
		require.Equal(t, "name", f.GetSortBy())
		require.Equal(t, "asc", f.GetSortOrder())
		return nil, 0, nil
	})
	status, _ := auditRequest(t, cfg, http.MethodGet,
		"/audit/data?unknown=value&private=value&name=Alice&sortBy=unknown&sortOrder=SIDEWAYS", "")
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, 1, calls)
}

func TestPublicPOSTKeepsConfiguredCustomFiltersAndSorts(t *testing.T) {
	for _, sortBy := range []string{"name", "rank", "id", "createdAt", "updated_at", "host_default"} {
		t.Run(sortBy, func(t *testing.T) {
			calls := 0
			wantFields := map[string]any{
				"custom": map[string]any{"tags": []any{"one", "two"}},
				"list":   []any{float64(1), float64(2)}, "name": nil,
			}
			cfg := auditConfig(func(_ context.Context, f datagrid.Filtered) ([]auditRow, int, error) {
				calls++
				require.Equal(t, sortBy, f.GetSortBy())
				require.Equal(t, "DESC", f.GetSortOrder())
				require.Equal(t, wantFields, f.GetFields())
				return nil, 0, nil
			})
			cfg.DefaultSort = "host_default" // Trusted defaults need not be visible sortable columns.
			cfg.Columns = append(cfg.Columns,
				datagrid.Column{Key: "custom", Type: "custom", Filterable: true},
				datagrid.Column{Key: "list", Filterable: true},
				datagrid.Column{Key: "rank", Sortable: true})
			body, err := json.Marshal(datagrid.Filters{SortBy: sortBy, SortOrder: "DESC", Fields: wantFields})
			require.NoError(t, err)
			status, _ := auditRequest(t, cfg, http.MethodPost, "/audit/data", string(body))
			require.Equal(t, http.StatusOK, status)
			require.Equal(t, 1, calls)
		})
	}
}

func TestPublicGETAndPOSTValidateTypedFilterOptions(t *testing.T) {
	for _, tc := range []struct {
		name, query, body string
		status            int
	}{
		{
			"valid scalars", "count=7&enabled=true&day=2026-10-02&status=active",
			`{"fields":{"count":7,"enabled":true,"day":"2026-10-02","status":"active"}}`, 200,
		},
		{"invalid number", "count=8", `{"fields":{"count":8}}`, 400},
		{"invalid boolean", "enabled=false", `{"fields":{"enabled":false}}`, 400},
		{"invalid date", "day=2026-10-03", `{"fields":{"day":"2026-10-03"}}`, 400},
		{"invalid select", "status=unknown", `{"fields":{"status":"unknown"}}`, 400},
	} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			t.Run(tc.name+"/"+method, func(t *testing.T) {
				calls := 0
				cfg := auditConfig(func(context.Context, datagrid.Filtered) ([]auditRow, int, error) {
					calls++
					return nil, 0, nil
				})
				options := map[string]string{"count": "7", "enabled": "true", "day": "2026-10-02"}
				for i := range cfg.Columns {
					if value, exists := options[cfg.Columns[i].Key]; exists {
						cfg.Columns[i].FilterOptions = []map[string]string{{"value": value}}
					}
				}
				path, body := "/audit/data?"+tc.query, ""
				if method == http.MethodPost {
					path, body = "/audit/data", tc.body
				}
				status, _ := auditRequest(t, cfg, method, path, body)
				require.Equal(t, tc.status, status)
				if tc.status == http.StatusOK {
					require.Equal(t, 1, calls)
				} else {
					require.Zero(t, calls)
				}
			})
		}
	}
}

func TestPublicGETAndPOSTNumericFilterOptions(t *testing.T) {
	decoders := map[string]func([]byte, any) error{
		"standard": json.Unmarshal,
		"runtime":  gojson.Unmarshal,
	}
	for decoderName, decoder := range decoders {
		for _, tc := range []struct {
			name, option, query, value string
			getStatus, postStatus      int
			getValue, postValue        any
		}{
			{"whole million", "1000000", "1000000", "1000000", 200, 200, 1000000, float64(1000000)},
			{"JSON exponent", "1000000", "1000000", "1e6", 200, 200, 1000000, float64(1000000)},
			{"existing exponent option", "1e+06", "1000000", "1e6", 400, 200, nil, float64(1000000)},
			{"tiny fraction", "0.000001", "0.000001", "0.000001", 200, 200, nil, float64(0.000001)},
			{"tiny exponent", "0.000001", "1e-6", "1e-6", 200, 200, nil, float64(0.000001)},
			{"invalid whole", "1000000", "1000001", "1000001", 400, 400, nil, nil},
			{"invalid fraction", "0.000001", "0.000002", "0.000002", 200, 400, nil, nil},
			{"exact identifier", "001", "001", `"001"`, 400, 200, nil, "001"},
			{"no identifier coercion", "001", "1", "1", 400, 400, nil, nil},
			{
				"positive safe boundary", "9007199254740991", "9007199254740991", "9007199254740991",
				200, 200, 9007199254740991, float64(9007199254740991),
			},
			{
				"negative safe boundary", "-9007199254740991", "-9007199254740991", "-9007199254740991",
				200, 200, -9007199254740991, float64(-9007199254740991),
			},
			{
				"positive unsafe boundary", "9007199254740992", "9007199254740992", "9007199254740992",
				200, 400, 9007199254740992, nil,
			},
			{
				"negative unsafe boundary", "-9007199254740992", "-9007199254740992", "-9007199254740992",
				200, 400, -9007199254740992, nil,
			},
			{
				"positive rounded neighbor", "9007199254740992", "9007199254740993", "9007199254740993",
				400, 400, nil, nil,
			},
			{
				"negative rounded neighbor", "-9007199254740992", "-9007199254740993", "-9007199254740993",
				400, 400, nil, nil,
			},
		} {
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				t.Run(decoderName+"/"+tc.name+"/"+method, func(t *testing.T) {
					wantStatus, wantValue := tc.getStatus, tc.getValue
					path, body := "/audit/data?count="+tc.query, ""
					if method == http.MethodPost {
						wantStatus, wantValue = tc.postStatus, tc.postValue
						path, body = "/audit/data", `{"fields":{"count":`+tc.value+`}}`
					}
					calls := 0
					cfg := auditConfig(func(_ context.Context, f datagrid.Filtered) ([]auditRow, int, error) {
						calls++
						if wantValue == nil {
							require.NotContains(t, f.GetFields(), "count")
						} else {
							require.Equal(t, wantValue, f.GetFields()["count"])
						}
						return nil, 0, nil
					})
					cfg.Columns = []datagrid.Column{
						{
							Key: "count", Type: "number", Filterable: true,
							FilterOptions: []map[string]string{{"value": tc.option}},
						},
					}
					app := fiber.New(fiber.Config{JSONDecoder: decoder})
					datagrid.NewHandler(cfg, logger.Discard()).RegisterRoutes(app, "/audit")
					req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
					req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
					resp, err := app.Test(req)
					require.NoError(t, err)
					require.NoError(t, resp.Body.Close())
					require.Equal(t, wantStatus, resp.StatusCode)
					if wantStatus == http.StatusOK {
						require.Equal(t, 1, calls)
					} else {
						require.Zero(t, calls)
					}
				})
			}
		}
	}
}

func TestPublicNumericOptionFallbackKeepsScalarBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, columnType, option string
		value                    any
		valid                    bool
	}{
		{"float32 decimal", "number", "1000000", float32(1000000), true},
		{"float32 tiny decimal", "number", "0.000001", float32(0.000001), true},
		{"float32 positive safe", "number", "16777215", float32(16777215), true},
		{"float32 negative safe", "number", "-16777215", float32(-16777215), true},
		{"float32 positive unsafe", "number", "16777216", float32(16777216), false},
		{"float32 negative unsafe", "number", "-16777216", float32(-16777216), false},
		{"NaN", "number", "NaN", math.NaN(), false},
		{"positive infinity", "number", "+Inf", math.Inf(1), false},
		{"negative infinity", "number", "-Inf", math.Inf(-1), false},
		{"unsafe exact exponent", "number", "9.007199254740992e+15", float64(9007199254740992), false},
		{"string identifier", "select", "001", "001", true},
		{"string mismatch", "select", "001", "1", false},
		{"boolean", "boolean", "true", true, true},
		{"custom scalar", "custom", "1e+06", float64(1000000), true},
		{"no custom coercion", "custom", "1000000", float64(1000000), false},
		{"number strings stay exact", "number", "1000000", "1e6", false},
		{"json Number stays unsupported", "number", "1000000", json.Number("1000000"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := datagrid.FilterConfig[any]{
				Columns: []datagrid.Column{{
					Key: "value", Type: tc.columnType, Filterable: true,
					FilterOptions: []map[string]string{{"value": tc.option}},
				}},
			}
			filters := datagrid.Filters{Page: 1, Limit: 25, Fields: map[string]any{"value": tc.value}}
			validationErrors := filters.Validate(cfg)
			if tc.valid {
				require.Empty(t, validationErrors)
			} else {
				require.Equal(t, []string{"Недопустимое значение для фильтра 'value'"}, validationErrors)
			}
		})
	}
}
