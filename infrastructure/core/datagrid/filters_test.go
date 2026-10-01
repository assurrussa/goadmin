package datagrid_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/infrastructure/core/datagrid"
)

func TestParseFilters(t *testing.T) {
	config := datagrid.FilterConfig[any]{
		Entity:       "test",      //nolint:goconst // required
		DefaultSort:  "createdAt", //nolint:goconst // required
		DefaultOrder: "desc",      //nolint:goconst // required
		PageSize:     10,
		Columns: []datagrid.Column{
			{Key: "status", Type: "badge", Sortable: false, Filterable: true}, //nolint:goconst // required
			{Key: "name", Type: "text", Sortable: true, Filterable: true},     //nolint:goconst // required
			{Key: "count", Type: "number", Sortable: true, Filterable: true},
			{Key: "active", Type: "boolean", Sortable: false, Filterable: true},      //nolint:goconst // required
			{Key: "date", Type: "date", Sortable: true, Filterable: true},            //nolint:goconst // required
			{Key: "read" + "only", Type: "text", Sortable: false, Filterable: false}, // не фильтруемая колонка
		},
	}

	app := fiber.New()
	app.Get("/test", func(c fiber.Ctx) error {
		filters := datagrid.ParseFilters(c, config)
		return c.JSON(filters)
	})

	// Тест с пустыми параметрами
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()
	assert.Equal(t, 200, resp.StatusCode)

	// Тест с полными параметрами
	req = httptest.NewRequestWithContext(context.Background(), http.MethodGet,
		"/test?page=2&limit=20&search=test&sortBy=name&sortOrder=asc&status=active&name=john&count=42&active=true&date=2023-01-01&readonly=ignored", //nolint:lll // required
		nil)

	resp, err = app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()
	assert.Equal(t, 200, resp.StatusCode)

	// Тест с невалидными параметрами
	req = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test?page=invalid&limit=invalid&count=invalid&active=invalid&date=invalid", nil) //nolint:lll // tests
	resp, err = app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()
	assert.Equal(t, 200, resp.StatusCode)
}

func TestIsValidSortField(t *testing.T) {
	config := datagrid.FilterConfig[any]{
		Columns: []datagrid.Column{
			{Key: "name", Type: "text", Sortable: true, Filterable: true},
			{Key: "status", Type: "badge", Sortable: false, Filterable: true},
			{Key: "read" + "only", Type: "text", Sortable: false, Filterable: false},
		},
	}

	// Базовые поля
	assert.True(t, datagrid.IsValidSortField("id", config))
	assert.True(t, datagrid.IsValidSortField("createdAt", config))
	assert.True(t, datagrid.IsValidSortField("updated_at", config))

	// Кастомные поля
	assert.True(t, datagrid.IsValidSortField("name", config))         // sortable = true
	assert.False(t, datagrid.IsValidSortField("status", config))      // sortable = false
	assert.False(t, datagrid.IsValidSortField("read"+"only", config)) // sortable = false
	assert.False(t, datagrid.IsValidSortField("unknown", config))     // не существует
}

func TestFilters_ToQueryParams(t *testing.T) {
	config := datagrid.FilterConfig[any]{
		DefaultSort:  "createdAt",
		DefaultOrder: "desc",
		PageSize:     10,
	}

	filters := datagrid.Filters{
		Page:      2,
		Limit:     20,
		Search:    "test",
		SortBy:    "name",
		SortOrder: "asc", //nolint:goconst // required
		Fields: map[string]any{
			"status": "active",
			"count":  42,
			"active": true,
			"date":   time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	params := filters.ToQueryParams(config)

	assert.Equal(t, "2", params.Get("page"))
	assert.Equal(t, "20", params.Get("limit"))
	assert.Equal(t, "test", params.Get("search"))
	assert.Equal(t, "name", params.Get("sortBy"))
	assert.Equal(t, "asc", params.Get("sortOrder"))
	assert.Equal(t, "active", params.Get("status"))
	assert.Equal(t, "42", params.Get("count"))
	assert.Equal(t, "true", params.Get("active"))
	assert.Equal(t, "2023-01-01", params.Get("date"))
}

func TestFilters_ToQueryParams_Defaults(t *testing.T) {
	config := datagrid.FilterConfig[any]{
		DefaultSort:  "createdAt",
		DefaultOrder: "desc",
		PageSize:     10,
	}

	filters := datagrid.Filters{
		Page:      1,
		Limit:     10,
		Search:    "",
		SortBy:    "createdAt",
		SortOrder: "desc",
		Fields:    map[string]any{},
	}

	params := filters.ToQueryParams(config)

	// Значения по умолчанию не должны добавляться
	assert.Empty(t, params.Get("page"))
	assert.Empty(t, params.Get("limit"))
	assert.Empty(t, params.Get("search"))
	assert.Empty(t, params.Get("sortBy"))
	assert.Empty(t, params.Get("sortOrder"))
}

func TestFilters_GetOffset(t *testing.T) {
	tests := []struct {
		page     int
		limit    int
		expected int
	}{
		{1, 10, 0},
		{2, 10, 10},
		{3, 20, 40},
		{0, 10, -10}, // Некорректная страница
	}

	for _, tt := range tests {
		filters := datagrid.Filters{Page: tt.page, Limit: tt.limit}
		assert.Equal(t, tt.expected, filters.GetOffset())
	}
}

func TestFilters_Validate(t *testing.T) {
	config := datagrid.FilterConfig[any]{
		Columns: []datagrid.Column{
			{
				Key:        "status",
				Type:       "select", //nolint:goconst // required
				Filterable: true,
				FilterOptions: []map[string]string{
					{"value": "active", "label": "Активный"},     //nolint:goconst // required
					{"value": "inactive", "label": "Неактивный"}, //nolint:goconst // required
				},
			},
			{
				Key:        "name",
				Type:       "text",
				Filterable: true,
				// Без FilterOptions - любое значение валидно
			},
		},
	}

	// Тест валидации пагинации
	filters := datagrid.Filters{Page: 0, Limit: 0}
	errors := filters.Validate(config)
	assert.Contains(t, errors, "Страница должна быть больше 0")
	assert.Contains(t, errors, "Лимит должен быть от 1 до 1000")

	// Тест валидации пагинации
	filters = datagrid.Filters{Page: -1, Limit: 9999999}
	errors = filters.Validate(config)
	assert.Contains(t, errors, "Страница должна быть больше 0")
	assert.Contains(t, errors, "Лимит должен быть от 1 до 1000")

	// Тест валидации select фильтра с невалидным значением
	filters = datagrid.Filters{
		Page:  1,
		Limit: 10,
		Fields: map[string]any{
			"status": "invalid_value",
		},
	}
	errors = filters.Validate(config)
	assert.Contains(t, errors, "Недопустимое значение для фильтра 'status'")

	// Тест валидного select фильтра
	filters = datagrid.Filters{
		Page:  1,
		Limit: 10,
		Fields: map[string]any{
			"status": "active",
		},
	}
	errors = filters.Validate(config)
	assert.Empty(t, errors)

	// Тест text фильтра без ограничений
	filters = datagrid.Filters{
		Page:  1,
		Limit: 10,
		Fields: map[string]any{
			"name": "любое значение",
		},
	}
	errors = filters.Validate(config)
	assert.Empty(t, errors)
}

func TestFilters_BuildURL(t *testing.T) {
	config := datagrid.FilterConfig[any]{
		DefaultSort:  "createdAt",
		DefaultOrder: "desc",
		PageSize:     10,
	}

	// Тест с пустыми фильтрами
	filters := datagrid.Filters{
		Page:   1,
		Limit:  10,
		Fields: map[string]any{},
	}
	url := filters.BuildURL("/test", config)
	assert.Equal(t, "/test", url)

	// Тест с фильтрами
	filters = datagrid.Filters{
		Page:      2,
		Limit:     20,
		Search:    "test",
		SortBy:    "name",
		SortOrder: "asc",
		Fields: map[string]any{
			"status": "active",
		},
	}
	url = filters.BuildURL("/test", config)
	assert.Contains(t, url, "page=2")
	assert.Contains(t, url, "limit=20")
	assert.Contains(t, url, "search=test")
	assert.Contains(t, url, "sortBy=name")
	assert.Contains(t, url, "sortOrder=asc")
	assert.Contains(t, url, "status=active")
}

func TestParseFilters_DefaultPageSize(t *testing.T) {
	// Тест с конфигурацией без PageSize
	config := datagrid.FilterConfig[any]{
		Entity: "test",
	}

	app := fiber.New()
	app.Get("/test", func(c fiber.Ctx) error {
		filters := datagrid.ParseFilters(c, config)
		return c.JSON(filters)
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()
	assert.Equal(t, 200, resp.StatusCode)
}

func TestParseFilters_OnlyFilterableColumns(t *testing.T) {
	// Тест что парсятся только фильтруемые колонки
	config := datagrid.FilterConfig[any]{
		Entity:   "test",
		PageSize: 10,
		Columns: []datagrid.Column{
			{Key: "filterable", Type: "text", Filterable: true},
			{Key: "read" + "only", Type: "text", Filterable: false},
		},
	}

	app := fiber.New()
	app.Get("/test", func(c fiber.Ctx) error {
		filters := datagrid.ParseFilters(c, config)

		// Проверяем что только фильтруемые поля попали в Fields
		_, hasFilterable := filters.Fields["filterable"]
		_, hasReadonly := filters.Fields["read"+"only"]

		return c.JSON(map[string]any{
			"has_filterable": hasFilterable,
			"has_readonly":   hasReadonly,
		})
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test?filterable=value1&readonly=value2", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()
	assert.Equal(t, 200, resp.StatusCode)
}

func TestGetFilterableColumns(t *testing.T) {
	config := datagrid.FilterConfig[any]{
		Columns: []datagrid.Column{
			{Key: "name", Filterable: true},
			{Key: "status", Filterable: true},
			{Key: "read" + "only", Filterable: false},
		},
	}

	filterable := config.GetFilterableColumns()
	assert.Len(t, filterable, 2)
	assert.Equal(t, "name", filterable[0].Key)
	assert.Equal(t, "status", filterable[1].Key)
}

func TestGetSortableColumns(t *testing.T) {
	config := datagrid.FilterConfig[any]{
		Columns: []datagrid.Column{
			{Key: "name", Sortable: true},
			{Key: "createdAt", Sortable: true},
			{Key: "read" + "only", Sortable: false},
		},
	}

	sortable := config.GetSortableColumns()
	assert.Len(t, sortable, 2)
	assert.Equal(t, "name", sortable[0].Key)
	assert.Equal(t, "createdAt", sortable[1].Key)
}
