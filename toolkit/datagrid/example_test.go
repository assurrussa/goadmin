//nolint:goconst // Keep the standalone example readable without coupling it to other test fixtures.
package datagrid_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	logger "github.com/assurrussa/gologger"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

type exampleUser struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`

	// Request-owned enrichment is excluded from the original JSON item.
	teamName string
}

type exampleRepository struct{}

var errExampleUnavailable = errors.New("private database connection detail")

var _ datagrid.Repository[exampleUser] = exampleRepository{}

func (exampleRepository) GetList(_ context.Context, filters datagrid.Filtered) ([]exampleUser, int, error) {
	if filters.GetSearch() == "unavailable" {
		return nil, 0, errExampleUnavailable
	}
	// Return a fresh slice for every request: Prepare below enriches these rows.
	rows := []exampleUser{{ID: 1, Name: "Ada", Active: true}, {ID: 2, Name: "Grace"}}
	selected := make([]exampleUser, 0, len(rows))
	for _, row := range rows {
		if strings.Contains(strings.ToLower(row.Name), strings.ToLower(filters.GetSearch())) {
			selected = append(selected, row)
		}
	}
	// This tiny repository supports search and pagination only. Real repositories
	// must explicitly allowlist sort/field names and bind SQL values themselves.
	total := len(selected)
	page, limit := filters.GetPage(), filters.GetLimit()
	if page < 1 || limit < 1 {
		return nil, 0, errors.New("invalid pagination")
	}
	// Compare before multiplying so an enormous positive page cannot overflow.
	start := total
	if page-1 <= total/limit {
		start = min((page-1)*limit, total)
	}
	end := start + min(limit, total-start)
	return selected[start:end], total, nil
}

// ExampleNewHandler exercises the supported public API without a database,
// listening socket, internal goadmin imports, or an admin runtime.
func ExampleNewHandler() {
	prepare := func(_ context.Context, rows []exampleUser) error {
		// Replace this fixture with ONE batch lookup using the page's row IDs.
		teams := map[int]string{1: "Engineering", 2: "Operations"}
		fmt.Println("batch rows:", len(rows))
		for i := range rows {
			rows[i].teamName = teams[rows[i].ID]
		}
		return nil
	}
	resolve := func(_ context.Context, row exampleUser) (map[string]any, error) {
		return map[string]any{"team": row.teamName}, nil
	}
	config := datagrid.FilterConfig[exampleUser]{
		RoutePath: "/users", Entity: "users", Title: "Users",
		Repository: exampleRepository{}, PageSize: 10,
		DefaultSort: "id", DefaultOrder: "asc", IDKey: "id",
		Columns: []datagrid.Column{
			{Key: "name", Label: "Name", Type: "text"},
			{Key: "team", Label: "Team", Type: "text"},
		},
		Actions: []datagrid.Action[exampleUser]{
			{Key: "view", Label: "View"},
			{Key: "edit", Label: "Edit", CanView: func(_ context.Context, row exampleUser) bool { return row.Active }},
		},
		RowDecorators: []datagrid.RowDecorator[exampleUser]{datagrid.NewRowDecorator(prepare, resolve)},
		PageComponent: func(c fiber.Ctx, data datagrid.Response[exampleUser]) error {
			// A real host can render its page template here instead.
			return c.SendString(fmt.Sprintf("%s: %d rows", data.Title, len(data.Items)))
		},
		ErrorComponent: func(c fiber.Ctx, data datagrid.ErrorData, err error) error {
			// The page callback receives the original error. Never expose it to clients.
			fmt.Println("page error:", errors.Is(err, errExampleUnavailable), data.Status)
			return c.Status(http.StatusServiceUnavailable).SendString("Users temporarily unavailable")
		},
	}
	app := fiber.New()
	// In production, register on a router protected by host authentication/RBAC.
	datagrid.NewHandler(config, logger.Discard()).RegisterRoutes(app, "/users")

	response, err := app.Test(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/users/data", nil))
	if err != nil {
		panic(err)
	}
	defer response.Body.Close()
	var data datagrid.APIResponse[exampleUser]
	if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
		panic(err)
	}
	fmt.Println("API:", response.StatusCode, data.Meta.Pagination.Total)
	for _, row := range data.Data {
		fmt.Println(row.Item.Name, row.Values["team"], len(row.Actions))
	}
	for _, path := range []string{"/users?search=Ada", "/users?search=unavailable", "/users/data?search=unavailable"} {
		response, err := app.Test(httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil))
		if err != nil {
			panic(err)
		}
		body, err := io.ReadAll(response.Body)
		if err != nil {
			panic(err)
		}
		if err := response.Body.Close(); err != nil {
			panic(err)
		}
		fmt.Println(path, response.StatusCode, string(body))
	}
	// Output:
	// batch rows: 2
	// API: 200 2
	// Ada Engineering 2
	// Grace Operations 1
	// batch rows: 1
	// /users?search=Ada 200 Users: 1 rows
	// page error: true 500
	// /users?search=unavailable 503 Users temporarily unavailable
	// /users/data?search=unavailable 500 Internal Server Error
}

func TestExampleRepositoryPagination(t *testing.T) {
	for _, tc := range []struct {
		page, limit, wantID int
	}{
		{1, 1, 1}, {2, 1, 2}, {3, 1, 0}, {int(^uint(0) >> 1), 10, 0},
	} {
		rows, total, err := (exampleRepository{}).GetList(context.Background(), datagrid.Filters{Page: tc.page, Limit: tc.limit})
		require.NoError(t, err)
		require.Equal(t, 2, total)
		if tc.wantID == 0 {
			require.Empty(t, rows)
		} else {
			require.Len(t, rows, 1)
			require.Equal(t, tc.wantID, rows[0].ID)
		}
	}
}
