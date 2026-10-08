//nolint:goconst // Contract fixtures intentionally repeat wire values.
package datagrid_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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
