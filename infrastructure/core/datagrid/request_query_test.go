package datagrid_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	logger "github.com/assurrussa/gologger"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	datagrid "github.com/assurrussa/goadmin/infrastructure/core/datagrid"
)

type queryCountRepository struct {
	MockRepository
	calls int
}

func (r *queryCountRepository) GetList(ctx context.Context, filters datagrid.Filtered) ([]TestEntity, int, error) {
	r.calls++
	return r.MockRepository.GetList(ctx, filters)
}

func TestResponseRequestQueryProvenance(t *testing.T) {
	const (
		defaultQueryOrder = "desc"
		queryTestEntity   = "test"
	)
	queries := []string{
		"", "limit=1000",
		"page=2&limit=1&sortBy=name&sortOrder=asc&name=Alice%20%26%20Bob",
		"sortBy=unknown",
	}
	for _, query := range queries {
		t.Run(query, func(t *testing.T) {
			repository := &queryCountRepository{MockRepository: MockRepository{data: []TestEntity{{ID: 1, Name: "Alice"}}, total: 1}}
			config := datagrid.FilterConfig[TestEntity]{
				Entity: queryTestEntity, Title: "Test", RoutePath: "/users", PageSize: 20,
				DefaultSort: "id", DefaultOrder: defaultQueryOrder, Repository: repository,
				Columns: []datagrid.Column{
					{Key: "id", Sortable: true}, {Key: testNameField, Sortable: true, Filterable: true},
				},
			}
			config.PageComponent = func(c fiber.Ctx, response datagrid.Response[TestEntity]) error {
				return c.JSON(response.ToAPIResponse())
			}
			handler := datagrid.NewHandler(config, logger.Discard())
			app := fiber.New()
			app.Get("/users", handler.HandlePage)
			app.Get("/users/data", handler.HandleData)
			for _, path := range []string{"/users", "/users/data"} {
				repository.calls = 0
				response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, path+"?"+query, nil))
				require.NoError(t, err)
				body, err := io.ReadAll(response.Body)
				require.NoError(t, err)
				require.NoError(t, response.Body.Close())
				require.Equal(t, http.StatusOK, response.StatusCode, string(body))
				var result datagrid.APIResponse[TestEntity]
				require.NoError(t, json.Unmarshal(body, &result))
				require.NotNil(t, result.Meta.RequestQuery)
				require.Equal(t, query, *result.Meta.RequestQuery)
				require.Equal(t, 1, repository.calls)
				if query == "limit=1000" {
					require.Equal(t, 20, result.Meta.Pagination.PerPage)
				}
				if query == "sortBy=unknown" {
					require.Equal(t, "id", result.Meta.Sorting.SortBy)
				}
			}
		})
	}
	// Manual/legacy responses cannot claim an applied query that the server did not observe.
	result := datagrid.Response[TestEntity]{}.ToAPIResponse()
	require.Nil(t, result.Meta.RequestQuery)
}
