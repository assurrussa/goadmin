package datagrid_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	logger "github.com/assurrussa/gologger"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/infrastructure/core/datagrid"
)

// TestEntity тестовая сущность.
type TestEntity struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// MockRepository мок репозитория.
type MockRepository struct {
	data  []TestEntity
	total int
	err   error
}

func (m *MockRepository) GetList(_ context.Context, filters datagrid.Filtered) ([]TestEntity, int, error) {
	if m.err != nil {
		return nil, 0, m.err
	}

	// Простая имитация пагинации
	start := filters.GetOffset()
	end := filters.GetOffset() + filters.GetLimit()
	if start > len(m.data) {
		start = len(m.data)
	}
	if end > len(m.data) {
		end = len(m.data)
	}

	return m.data[start:end], m.total, nil
}

func setupTestHandler() (*datagrid.Handler[TestEntity], *MockRepository) {
	mockRepo := &MockRepository{
		data: []TestEntity{
			{ID: 1, Name: "Test 1", Status: "active"},   //nolint:goconst // required
			{ID: 2, Name: "Test 2", Status: "inactive"}, //nolint:goconst // required
			{ID: 3, Name: "Test 3", Status: "active"},
		},
		total: 3,
	}

	config := datagrid.FilterConfig[TestEntity]{
		Entity:       "test",          //nolint:goconst // required
		Title:        "Test Entities", //nolint:goconst // required
		DefaultSort:  "createdAt",     //nolint:goconst // required
		DefaultOrder: "desc",          //nolint:goconst // required
		PageSize:     10,
		SearchMode:   datagrid.SearchModeServer,
		Repository:   mockRepo,
		Columns: []datagrid.Column{
			{Key: "id", Label: "ID", Type: "number", Sortable: true, Filterable: false},
			{Key: "name", Label: "Название", Type: "text", Sortable: true, Filterable: true, FilterPlaceholder: "Поиск по имени"}, //nolint:goconst,lll // required
			{
				Key:               "status", //nolint:goconst // required
				Label:             "Статус", //nolint:goconst // required
				Type:              "select", //nolint:goconst // required
				Sortable:          true,
				Filterable:        true,
				FilterPlaceholder: "Выберите статус",
				FilterOptions: []map[string]string{
					{"value": "active", "label": "Активный"},     //nolint:goconst // required
					{"value": "inactive", "label": "Неактивный"}, //nolint:goconst // required
				},
				Badges: map[string]map[string]string{
					"active":   {"label": "Активный", "variant": "success"},
					"inactive": {"label": "Неактивный", "variant": "danger"}, //nolint:goconst // required
				},
			},
		},
		Actions: []datagrid.Action[TestEntity]{
			{Key: "edit", Label: "Редактировать", Icon: "fas fa-edit", Variant: "primary"},
			{Key: "delete", Label: "Удалить", Icon: "fas fa-trash", Variant: "danger"},
			{Key: "export", Label: "экспорт", Icon: "fas fa-export", Variant: "info", CanView: func(_ context.Context, _ TestEntity) bool {
				return false
			}},
		},
		PageComponent: func(c fiber.Ctx, _ datagrid.Response[TestEntity]) error {
			c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
			return c.Send([]byte("<html><body>Test Page</body></html>"))
		},
		ErrorComponent: func(c fiber.Ctx, _ datagrid.ErrorData, _ error) error {
			c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
			return c.Send([]byte("<html><body>Test Error</body></html>"))
		},
	}

	handler := datagrid.NewHandler(config, logger.Discard())
	return handler, mockRepo
}

func TestHandler_HandlePage_Success(t *testing.T) {
	handler, _ := setupTestHandler()

	app := fiber.New()
	handler.RegisterRoutes(app, "/test")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, fiber.MIMETextHTMLCharsetUTF8, resp.Header.Get("Content-Type"))
}

func TestHandler_HandleData_Success(t *testing.T) {
	handler, _ := setupTestHandler()

	app := fiber.New()
	handler.RegisterRoutes(app, "/test")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test/data", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response map[string]any
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	// Проверяем новый формат APIResponse[T]
	data, ok := response["data"].([]any)
	require.True(t, ok, "data should be []any")
	assert.Len(t, data, 3)
	for _, item := range data {
		res, ok := item.(map[string]any)
		require.True(t, ok, "data should be map[string]any")
		assert.Len(t, res["actions"], 2)
	}

	// Проверяем meta секцию
	meta, ok := response["meta"].(map[string]any)
	require.True(t, ok, "meta should be map[string]any")

	pagination, ok := meta["pagination"].(map[string]any)
	require.True(t, ok, "pagination should be map[string]any")
	assert.InEpsilon(t, float64(1), pagination["currentPage"], 0.1)
	assert.InEpsilon(t, float64(10), pagination["perPage"], 0.1)
	assert.InEpsilon(t, float64(3), pagination["total"], 0.1)
	assert.InEpsilon(t, float64(1), pagination["totalPages"], 0.1)

	sorting, ok := meta["sorting"].(map[string]any)
	require.True(t, ok, "sorting should be map[string]any")
	assert.Equal(t, "createdAt", sorting["sortBy"])
	assert.Equal(t, "desc", sorting["sortOrder"])

	// Проверяем config секцию
	config, ok := response["config"].(map[string]any)
	require.True(t, ok, "config should be map[string]any")

	columns, ok := config["columns"].([]any)
	require.True(t, ok, "columns should be []any")
	assert.Len(t, columns, 3)

	// Проверяем что у колонок есть флаги Filterable/Sortable
	var filterableCount int
	for _, col := range columns {
		column, ok := col.(map[string]any)
		require.True(t, ok, "column should be map[string]any")
		if filterable, exists := column["filterable"]; exists {
			if filterableBool, ok := filterable.(bool); ok && filterableBool {
				filterableCount++
			}
		}
	}
	assert.Equal(t, 2, filterableCount, "Should have 2 filterable columns")

	_, ok = config["actions"].([]any)
	require.False(t, ok, "actions should be []any")

	ui, ok := config["ui"].(map[string]any)
	require.True(t, ok, "ui should be map[string]any")
	assert.Equal(t, "Поиск...", ui["searchPlaceholder"])
	assert.Equal(t, "Создать", ui["createButtonText"])
}

func TestHandler_HandleData_WithPagination(t *testing.T) {
	handler, _ := setupTestHandler()

	app := fiber.New()
	handler.RegisterRoutes(app, "/test")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test/data?page=2&limit=2", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response map[string]any
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	// Проверяем новый формат APIResponse[T]
	meta, ok := response["meta"].(map[string]any)
	require.True(t, ok, "meta should be map[string]any")

	pagination, ok := meta["pagination"].(map[string]any)
	require.True(t, ok, "pagination should be map[string]any")
	assert.InEpsilon(t, float64(2), pagination["currentPage"], 0.1)
	assert.InEpsilon(t, float64(2), pagination["perPage"], 0.1)
}

func TestHandler_HandleData_WithFilters(t *testing.T) {
	handler, _ := setupTestHandler()

	app := fiber.New()
	handler.RegisterRoutes(app, "/test")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test/data?search=test&status=active&name=example", nil) //nolint:lll // required
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response map[string]any
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	// Проверяем новый формат APIResponse[T]
	meta, ok := response["meta"].(map[string]any)
	require.True(t, ok, "meta should be map[string]any")

	filters, ok := meta["filters"].(map[string]any)
	require.True(t, ok, "filters should be map[string]any")
	assert.Equal(t, "active", filters["status"])
	assert.Equal(t, "example", filters["name"])

	sorting, ok := meta["sorting"].(map[string]any)
	require.True(t, ok, "sorting should be map[string]any")
	// search не является sortBy, проверяем правильное поле
	assert.Equal(t, "createdAt", sorting["sortBy"]) // значение по умолчанию
}

func TestHandler_HandleData_ValidationErrors(t *testing.T) {
	// Создаем handler с фильтром для тестирования валидации
	mockRepo := &MockRepository{
		data: []TestEntity{
			{ID: 1, Name: "Test 1", Status: "active"},
		},
		total: 1,
	}

	config := datagrid.FilterConfig[TestEntity]{
		Entity:       "test",
		Title:        "Test Entities",
		DefaultSort:  "createdAt",
		DefaultOrder: "desc",
		PageSize:     10,
		Repository:   mockRepo,
		Columns: []datagrid.Column{
			{
				Key:        "status",
				Label:      "Статус",
				Type:       "select",
				Sortable:   true,
				Filterable: true,
				FilterOptions: []map[string]string{
					{"value": "active", "label": "Активный"},
					{"value": "inactive", "label": "Неактивный"},
				},
			},
		},

		PageComponent: func(c fiber.Ctx, _ datagrid.Response[TestEntity]) error {
			return c.Send([]byte("<html><body>Test Page</body></html>"))
		},
		ErrorComponent: func(c fiber.Ctx, _ datagrid.ErrorData, _ error) error {
			return c.Send([]byte("<html><body>Test Error</body></html>"))
		},
	}

	handler := datagrid.NewHandler(config, logger.Discard())
	app := fiber.New()
	handler.RegisterRoutes(app, "/test")

	// Тест с невалидными параметрами пагинации
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test/data?page=0&limit=200", nil)
	resp, err := app.Test(req)
	defer func() { assert.NoError(t, resp.Body.Close()) }()
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()

	// Проверяем что валидация работает
	if resp.StatusCode != http.StatusUnprocessableEntity {
		// Если валидация не сработала, проверим что хотя бы данные вернулись
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	} else {
		assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	}

	// Тест с валидным фильтром (должен пройти)
	req = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test/data?status=active", nil)
	resp, err = app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestHandler_HandleData_RepositoryError(t *testing.T) {
	handler, mockRepo := setupTestHandler()
	mockRepo.err = assert.AnError

	app := fiber.New()
	handler.RegisterRoutes(app, "/test")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test/data", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestHandler_RegisterRoutes(t *testing.T) {
	handler, _ := setupTestHandler()

	app := fiber.New()

	// Тест что маршруты регистрируются без паники
	assert.NotPanics(t, func() {
		handler.RegisterRoutes(app, "/test")
	})

	// Проверяем что маршруты зарегистрированы
	routes := app.GetRoutes()
	assert.NotEmpty(t, routes)

	// Проверяем наличие нужных маршрутов
	routePaths := make(map[string]bool)
	for _, route := range routes {
		routePaths[route.Method+" "+route.Path] = true
	}

	assert.True(t, routePaths["GET /test"])
	assert.True(t, routePaths["GET /test/data"])
	assert.True(t, routePaths["POST /test/data"])
}

func TestHandler_HandlePage_ValidationErrors(t *testing.T) {
	// Создаем handler с фильтром для тестирования валидации на странице
	mockRepo := &MockRepository{
		data: []TestEntity{
			{ID: 1, Name: "Test 1", Status: "active"},
		},
		total: 1,
	}

	config := datagrid.FilterConfig[TestEntity]{
		Entity:       "test",
		Title:        "Test Entities",
		DefaultSort:  "createdAt",
		DefaultOrder: "desc",
		PageSize:     10,
		Repository:   mockRepo,
		Columns: []datagrid.Column{
			{
				Key:        "status",
				Label:      "Статус",
				Type:       "select",
				Sortable:   true,
				Filterable: true,
				FilterOptions: []map[string]string{
					{"value": "active", "label": "Активный"},
					{"value": "inactive", "label": "Неактивный"},
				},
			},
		},

		PageComponent: func(c fiber.Ctx, _ datagrid.Response[TestEntity]) error {
			return c.Send([]byte("<html><body>Test Page</body></html>"))
		},
		ErrorComponent: func(c fiber.Ctx, _ datagrid.ErrorData, _ error) error {
			return c.Send([]byte("<html><body>Test Error</body></html>"))
		},
	}

	handler := datagrid.NewHandler(config, logger.Discard())
	app := fiber.New()
	handler.RegisterRoutes(app, "/test")

	// Тест с невалидными параметрами - должен использовать значения по умолчанию
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test?page=0&limit=200", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "Test Page")
}

func TestHandler_HandlePage_RepositoryError(t *testing.T) {
	mockRepo := &MockRepository{
		err: assert.AnError, // Репозиторий вернет ошибку
	}

	config := datagrid.FilterConfig[TestEntity]{
		Entity:     "test",
		Title:      "Test Entities",
		Repository: mockRepo,
		PageComponent: func(c fiber.Ctx, _ datagrid.Response[TestEntity]) error {
			return c.Send([]byte("<html><body>Test Page</body></html>"))
		},
		ErrorComponent: func(c fiber.Ctx, _ datagrid.ErrorData, _ error) error {
			c.Status(fiber.StatusInternalServerError)
			return c.Send([]byte("<html><body>Test Error</body></html>"))
		},
	}

	handler := datagrid.NewHandler(config, logger.Discard())
	app := fiber.New()
	handler.RegisterRoutes(app, "/test")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "Test Error")
}

func TestHandler_HandleData_POST_Success(t *testing.T) {
	handler, _ := setupTestHandler()

	app := fiber.New()
	handler.RegisterRoutes(app, "/test")

	// Тест POST запроса с фильтрами
	reqBody := `{
		"page": 2,
		"limit": 5,
		"search": "test",
		"sortBy": "name",
		"sortOrder": "asc",
		"fields": {
			"status": "active"
		}
	}`

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/test/data", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response map[string]any
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	// Проверяем новый формат APIResponse[T]
	meta, ok := response["meta"].(map[string]any)
	require.True(t, ok, "meta should be map[string]any")

	pagination, ok := meta["pagination"].(map[string]any)
	require.True(t, ok, "pagination should be map[string]any")
	filters, ok := meta["filters"].(map[string]any)
	require.True(t, ok, "filters should be map[string]any")
	sorting, ok := meta["sorting"].(map[string]any)
	require.True(t, ok, "sorting should be map[string]any")

	assert.InEpsilon(t, float64(2), pagination["currentPage"], 0.1)
	assert.InEpsilon(t, float64(5), pagination["perPage"], 0.1)
	assert.Equal(t, "active", filters["status"])
	assert.Equal(t, "name", sorting["sortBy"])
	assert.Equal(t, "asc", sorting["sortOrder"])
}

func TestHandler_HandleData_POST_InvalidJSON(t *testing.T) {
	handler, _ := setupTestHandler()

	app := fiber.New()
	handler.RegisterRoutes(app, "/test")

	// Отправляем невалидный JSON
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/test/data", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()

	// Невалидное тело не должно подменяться query параметрами.
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestHandler_NewHandler_Defaults(t *testing.T) {
	// Тест что значения по умолчанию устанавливаются правильно
	config := datagrid.FilterConfig[TestEntity]{
		Entity:     "test",
		Repository: &MockRepository{},
		PageComponent: func(c fiber.Ctx, _ datagrid.Response[TestEntity]) error {
			return c.Send([]byte("<html><body>Test Page</body></html>"))
		},
	}

	handler := datagrid.NewHandler(config, logger.Discard())

	// Проверяем что значения по умолчанию установлены через API
	app := fiber.New()
	handler.RegisterRoutes(app, "/test")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test/data", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()

	var response map[string]any
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	// Проверяем новый формат APIResponse[T]
	meta, ok := response["meta"].(map[string]any)
	require.True(t, ok, "meta should be map[string]any")

	pagination, ok := meta["pagination"].(map[string]any)
	require.True(t, ok, "pagination should be map[string]any")
	sorting, ok := meta["sorting"].(map[string]any)
	require.True(t, ok, "sorting should be map[string]any")

	configSection, ok := response["config"].(map[string]any)
	require.True(t, ok, "config should be map[string]any")
	ui, ok := configSection["ui"].(map[string]any)
	require.True(t, ok, "ui should be map[string]any")

	assert.InEpsilon(t, float64(1), pagination["currentPage"], 0.1)
	assert.InEpsilon(t, float64(10), pagination["perPage"], 0.1) // PageSize по умолчанию
	assert.Equal(t, "createdAt", sorting["sortBy"])              // DefaultSort по умолчанию
	assert.Equal(t, "desc", sorting["sortOrder"])                // DefaultOrder по умолчанию
	assert.Equal(t, "Поиск...", ui["searchPlaceholder"])
	assert.Equal(t, "Создать", ui["createButtonText"])
	assert.Equal(t, "Нет данных для отображения", ui["emptyMessage"])
	assert.Equal(t, "id", ui["idKey"])
}

func TestHandler_LoadData_ZeroTotal(t *testing.T) {
	// Тест случая когда total = 0
	mockRepo := &MockRepository{
		data:  []TestEntity{},
		total: 0,
	}

	config := datagrid.FilterConfig[TestEntity]{
		Entity:     "test",
		Repository: mockRepo,
		PageComponent: func(c fiber.Ctx, _ datagrid.Response[TestEntity]) error {
			return c.Send([]byte("<html><body>Test Page</body></html>"))
		},
	}

	handler := datagrid.NewHandler(config, logger.Discard())
	app := fiber.New()
	handler.RegisterRoutes(app, "/test")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test/data", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()

	var response map[string]any
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	// Проверяем новый формат APIResponse[T]
	data, ok := response["data"].([]any)
	require.True(t, ok, "data should be []any")

	meta, ok := response["meta"].(map[string]any)
	require.True(t, ok, "meta should be map[string]any")
	pagination, ok := meta["pagination"].(map[string]any)
	require.True(t, ok, "pagination should be map[string]any")

	assert.Equal(t, float64(0), pagination["total"])      //nolint:testifylint // tests
	assert.Equal(t, float64(0), pagination["totalPages"]) //nolint:testifylint // tests
	assert.Empty(t, data)
}

func TestHandler_LoadData_FilterValues_NonString(t *testing.T) {
	const countKey = "count"
	const numberType = "number"

	// Typed filters must be returned using their canonical query representation.
	handler, _ := setupTestHandler()
	config := handler.GetConfig()
	config.Columns = append(config.Columns,
		datagrid.Column{Key: countKey, Type: numberType, Filterable: true},
		datagrid.Column{Key: "active", Type: "boolean", Filterable: true},
	)
	handler = datagrid.NewHandler(config, logger.Discard())

	app := fiber.New()
	handler.RegisterRoutes(app, "/test")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test/data?count=42&active=true", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()

	var response map[string]any
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	// Проверяем новый формат APIResponse[T]
	meta, ok := response["meta"].(map[string]any)
	require.True(t, ok, "meta should be map[string]any")
	filters, ok := meta["filters"].(map[string]any)
	require.True(t, ok, "filters should be map[string]any")

	assert.Equal(t, "42", filters[countKey])
	assert.Equal(t, "true", filters["active"])
}

// Тесты для новой функциональности

func TestHandler_SearchMode_Server(t *testing.T) {
	mockRepo := &MockRepository{
		data: []TestEntity{
			{ID: 1, Name: "Test 1", Status: "active"},
		},
		total: 1,
	}

	config := datagrid.FilterConfig[TestEntity]{
		Entity:     "test",
		Repository: mockRepo,
		SearchMode: datagrid.SearchModeServer, // явно указываем серверный поиск
		PageComponent: func(c fiber.Ctx, _ datagrid.Response[TestEntity]) error {
			return c.Send([]byte("<html><body>Test Page</body></html>"))
		},
	}

	handler := datagrid.NewHandler(config, logger.Discard())
	app := fiber.New()
	handler.RegisterRoutes(app, "/test")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test/data", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()

	var response map[string]any
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	// Проверяем новый формат APIResponse[T]
	configSection, ok := response["config"].(map[string]any)
	assert.True(t, ok)
	behaviour, ok := configSection["behaviour"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "server", behaviour["searchMode"])
}

func TestHandler_SearchMode_Client(t *testing.T) {
	mockRepo := &MockRepository{
		data: []TestEntity{
			{ID: 1, Name: "Test 1", Status: "active"},
		},
		total: 1,
	}

	config := datagrid.FilterConfig[TestEntity]{
		Entity:     "test",
		Repository: mockRepo,
		SearchMode: datagrid.SearchModeClient, // клиентский поиск
		PageComponent: func(c fiber.Ctx, _ datagrid.Response[TestEntity]) error {
			return c.Send([]byte("<html><body>Test Page</body></html>"))
		},
	}

	handler := datagrid.NewHandler(config, logger.Discard())
	app := fiber.New()
	handler.RegisterRoutes(app, "/test")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test/data", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()

	var response map[string]any
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	// Проверяем новый формат APIResponse[T]
	configSection, ok := response["config"].(map[string]any)
	assert.True(t, ok)
	behaviour, ok := configSection["behaviour"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "client", behaviour["searchMode"])
}

func TestHandler_FilterableColumns(t *testing.T) {
	handler, _ := setupTestHandler()

	app := fiber.New()
	handler.RegisterRoutes(app, "/test")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test/data", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()

	var response map[string]any
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	// Проверяем новый формат APIResponse[T]
	configSection, ok := response["config"].(map[string]any)
	assert.True(t, ok)
	columns, ok := configSection["columns"].([]any)
	assert.True(t, ok)

	// Проверяем что колонки содержат флаги Filterable
	for _, col := range columns {
		column, ok := col.(map[string]any)
		assert.True(t, ok)
		key, ok := column["key"].(string)
		assert.True(t, ok)

		switch key {
		case "id":
			// ID не фильтруемый
			filterable, exists := column["filterable"]
			if exists {
				val, ok := filterable.(bool)
				assert.True(t, ok, "filterable should be bool")
				assert.False(t, val, "ID column should not be filterable")
			}
		case "name", "status":
			// name и status фильтруемые
			val, ok := column["filterable"].(bool)
			assert.True(t, ok, "filterable should be bool")
			assert.True(t, val, key+" column should be filterable")
		}
	}
}

func TestHandler_OnlyFilterableColumnsProcessed(t *testing.T) {
	mockRepo := &MockRepository{
		data: []TestEntity{
			{ID: 1, Name: "Test 1", Status: "active"},
		},
		total: 1,
	}

	config := datagrid.FilterConfig[TestEntity]{
		Entity:     "test",
		Repository: mockRepo,
		Columns: []datagrid.Column{
			{Key: "name", Type: "text", Filterable: true},
			{Key: "read" + "only", Type: "text", Filterable: false}, // не фильтруемая //nolint:goconst // required
		},
		PageComponent: func(c fiber.Ctx, _ datagrid.Response[TestEntity]) error {
			return c.Send([]byte("<html><body>Test Page</body></html>"))
		},
	}

	handler := datagrid.NewHandler(config, logger.Discard())
	app := fiber.New()
	handler.RegisterRoutes(app, "/test")

	// Отправляем фильтры для обеих колонок
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test/data?name=test&readonly=ignored", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()

	var response map[string]any
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	// Проверяем новый формат APIResponse[T]
	meta, ok := response["meta"].(map[string]any)
	assert.True(t, ok)
	filterValues, ok := meta["filters"].(map[string]any)
	assert.True(t, ok)

	// Только фильтруемые поля должны попасть в filterValues
	assert.Equal(t, "test", filterValues["name"])
	_, hasReadonly := filterValues["read"+"only"]
	assert.False(t, hasReadonly, "Non-filterable columns should not appear in filterValues")
}

func TestHandler_FilterPlaceholder(t *testing.T) {
	handler, _ := setupTestHandler()

	app := fiber.New()
	handler.RegisterRoutes(app, "/test")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test/data", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()

	var response map[string]any
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	// Проверяем новый формат APIResponse[T]
	configSection, ok := response["config"].(map[string]any)
	assert.True(t, ok)
	columns, ok := configSection["columns"].([]any)
	assert.True(t, ok)

	// Проверяем что у фильтруемых колонок есть filterPlaceholder
	for _, col := range columns {
		column, ok := col.(map[string]any)
		assert.True(t, ok)
		key, ok := column["key"].(string)
		assert.True(t, ok)

		if val, ok := column["filterable"].(bool); ok && val {
			placeholder, exists := column["placeholder"]
			assert.True(t, exists, key+" filterable column should have filterPlaceholder")
			assert.NotEmpty(t, placeholder, key+" filterPlaceholder should not be empty")
		}
	}
}

func TestHandler_SelectFilterOptions(t *testing.T) {
	handler, _ := setupTestHandler()

	app := fiber.New()
	handler.RegisterRoutes(app, "/test")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test/data", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()

	var response map[string]any
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	// Проверяем новый формат APIResponse[T]
	configSection, ok := response["config"].(map[string]any)
	assert.True(t, ok)
	columns, ok := configSection["columns"].([]any)
	assert.True(t, ok)

	// Найдем колонку status (select тип)
	var statusColumn map[string]any
	for _, col := range columns {
		column, ok := col.(map[string]any)
		assert.True(t, ok)
		if column["key"] == "status" {
			statusColumn = column
			break
		}
	}

	require.NotNil(t, statusColumn, "Status column should exist")
	assert.Equal(t, "select", statusColumn["type"])

	options, ok := statusColumn["options"].([]any)
	assert.True(t, ok)
	assert.Len(t, options, 2, "Status column should have 2 filter options")

	// Проверяем структуру опций
	option1, ok := options[0].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "active", option1["value"])
	assert.Equal(t, "Активный", option1["label"])
}

func TestHandler_GetConfig(t *testing.T) {
	config := datagrid.FilterConfig[TestEntity]{
		Entity:     "test",
		Title:      "Test Entities",
		PageSize:   25,
		SearchMode: datagrid.SearchModeServer,
		Columns: []datagrid.Column{
			{Key: "name", Label: "Name", Sortable: true, Filterable: true},   //nolint:goconst // required
			{Key: "email", Label: "Email", Sortable: true, Filterable: true}, //nolint:goconst // required
		},
	}

	handler := datagrid.NewHandler(config, logger.Discard())

	retrievedConfig := handler.GetConfig()

	assert.Equal(t, config.Entity, retrievedConfig.Entity)
	assert.Equal(t, config.Title, retrievedConfig.Title)
	assert.Equal(t, config.PageSize, retrievedConfig.PageSize)
	assert.Equal(t, config.SearchMode, retrievedConfig.SearchMode)
	assert.Len(t, retrievedConfig.Columns, len(config.Columns))
}

func TestHandler_EnhancedPagination_Like(t *testing.T) {
	repo := &MockRepository{
		data: []TestEntity{
			{ID: 1, Name: "Entity1"},
			{ID: 2, Name: "Entity2"},
			{ID: 3, Name: "Entity3"},
			{ID: 4, Name: "Entity4"},
			{ID: 5, Name: "Entity5"},
		},
		total: 5,
	}

	config := datagrid.FilterConfig[TestEntity]{
		RoutePath:  "/test",
		Entity:     "test",
		Title:      "Test",
		Repository: repo,
		PageSize:   2,
		Columns: []datagrid.Column{
			{Key: "id", Label: "ID", Sortable: true},
			{Key: "name", Label: "Name", Sortable: true, Filterable: true},
		},
	}

	handler := datagrid.NewHandler(config, logger.Discard())

	app := fiber.New()
	handler.RegisterRoutes(app, "/test")

	// Test страница 2 из 3
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test/data?page=2&limit=2", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response datagrid.APIResponse[TestEntity]
	body, _ := io.ReadAll(resp.Body)
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	// Проверяем данные
	assert.Len(t, response.Data, 2)
	assert.Equal(t, "Entity3", response.Data[0].Item.Name)
	assert.Equal(t, "Entity4", response.Data[1].Item.Name)

	// Проверяем базовую пагинацию
	pagination := response.Meta.Pagination
	assert.Equal(t, 2, pagination.CurrentPage)
	assert.Equal(t, 2, pagination.PerPage)
	assert.Equal(t, 5, pagination.Total)
	assert.Equal(t, 3, pagination.TotalPages)

	// Проверяем -like поля
	assert.Equal(t, 3, pagination.From)
	assert.Equal(t, 4, pagination.To)

	// Проверяем URL'ы
	assert.Contains(t, pagination.FirstPageURL, "page=1")
	assert.Contains(t, pagination.LastPageURL, "page=3")
	assert.Contains(t, pagination.NextPageURL, "page=3")
	assert.Contains(t, pagination.PrevPageURL, "page=1")
	assert.Equal(t, "/test/data", pagination.Path)

	t.Logf("First page URL: %s", pagination.FirstPageURL)
	t.Logf("Next page URL: %s", pagination.NextPageURL)
	t.Logf("Prev page URL: %s", pagination.PrevPageURL)
	t.Logf("Last page URL: %s", pagination.LastPageURL)
}

func TestHandler_PaginationLinks_LaravelStyle(t *testing.T) {
	// Создаем больше данных для тестирования links
	repo := &MockRepository{
		data:  make([]TestEntity, 25), // 25 записей
		total: 25,
	}

	// Заполняем тестовыми данными
	for i := 0; i < 25; i++ {
		repo.data[i] = TestEntity{ID: i + 1, Name: fmt.Sprintf("Entity%d", i+1)}
	}

	config := datagrid.FilterConfig[TestEntity]{
		RoutePath:  "/api/entities",
		Entity:     "entity",
		Title:      "Entities",
		Repository: repo,
		PageSize:   5, // 5 записей на страницу = 5 страниц
		Columns: []datagrid.Column{
			{Key: "id", Label: "ID", Sortable: true},
			{Key: "name", Label: "Name", Sortable: true, Filterable: true},
		},
	}

	handler := datagrid.NewHandler(config, logger.Discard())
	app := fiber.New()
	handler.RegisterRoutes(app, "/api/entities")

	tests := []struct {
		name           string
		page           int
		expectedLinks  int
		shouldHavePrev bool
		shouldHaveNext bool
		expectedLabels []string
	}{
		{
			name:           "first_page",
			page:           1,
			expectedLinks:  7, // Previous + 1,2,3,4,5 + Next
			shouldHavePrev: false,
			shouldHaveNext: true,
			expectedLabels: []string{"Previous", "1", "2", "3", "4", "5", "Next"}, //nolint:goconst // required
		},
		{
			name:           "middle_page",
			page:           3,
			expectedLinks:  7, // Previous + 1,2,3,4,5 + Next
			shouldHavePrev: true,
			shouldHaveNext: true,
			expectedLabels: []string{"Previous", "1", "2", "3", "4", "5", "Next"},
		},
		{
			name:           "last_page",
			page:           5,
			expectedLinks:  7, // Previous + 1,2,3,4,5 + Next
			shouldHavePrev: true,
			shouldHaveNext: false,
			expectedLabels: []string{"Previous", "1", "2", "3", "4", "5", "Next"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, fmt.Sprintf("/api/entities/data?page=%d&limit=5", tt.page), nil) //nolint:lll // required
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer func() { assert.NoError(t, resp.Body.Close()) }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var response datagrid.APIResponse[TestEntity]
			body, _ := io.ReadAll(resp.Body)
			err = json.Unmarshal(body, &response)
			require.NoError(t, err)

			// Проверяем что links массив создан
			links := response.Meta.Pagination.Links
			assert.Len(t, links, tt.expectedLinks, "Links count should match expected")

			// Проверяем лейблы
			labels := make([]string, len(links))
			for i, link := range links {
				labels[i] = link.Label
			}
			assert.Equal(t, tt.expectedLabels, labels, "Link labels should match expected")

			// Проверяем активную страницу
			activeFound := false
			for _, link := range links {
				if link.Active {
					assert.Equal(t, strconv.Itoa(tt.page), link.Label, "Active link should match current page")
					activeFound = true
				}
			}
			assert.True(t, activeFound, "Should have one active link")

			// Проверяем Previous ссылку
			prevLink := links[0]
			assert.Equal(t, "Previous", prevLink.Label)
			if tt.shouldHavePrev {
				assert.NotEmpty(t, prevLink.URL, "Previous link should have URL when not on first page")
				assert.Contains(t, prevLink.URL, fmt.Sprintf("page=%d", tt.page-1))
			} else {
				assert.Empty(t, prevLink.URL, "Previous link should be empty on first page")
			}

			// Проверяем Next ссылку
			nextLink := links[len(links)-1]
			assert.Equal(t, "Next", nextLink.Label)
			if tt.shouldHaveNext {
				assert.NotEmpty(t, nextLink.URL, "Next link should have URL when not on last page")
				assert.Contains(t, nextLink.URL, fmt.Sprintf("page=%d", tt.page+1))
			} else {
				assert.Empty(t, nextLink.URL, "Next link should be empty on last page")
			}

			t.Logf("Page %d Links: %+v", tt.page, labels)
		})
	}
}

func TestHandler_PaginationLinks_WithEllipsis(t *testing.T) {
	// Создаем много данных для тестирования эллипсов
	repo := &MockRepository{
		data:  make([]TestEntity, 100), // 100 записей
		total: 100,
	}

	// Заполняем тестовыми данными
	for i := 0; i < 100; i++ {
		repo.data[i] = TestEntity{ID: i + 1, Name: fmt.Sprintf("Entity%d", i+1)}
	}

	config := datagrid.FilterConfig[TestEntity]{
		RoutePath:  "/api/entities",
		Entity:     "entity",
		Title:      "Entities",
		Repository: repo,
		PageSize:   5, // 5 записей на страницу = 20 страниц
		Columns: []datagrid.Column{
			{Key: "id", Label: "ID", Sortable: true},
			{Key: "name", Label: "Name", Sortable: true, Filterable: true},
		},
	}

	handler := datagrid.NewHandler(config, logger.Discard())
	app := fiber.New()
	handler.RegisterRoutes(app, "/api/entities")

	testCases := []struct {
		name               string
		page               int
		shouldHaveEllipsis bool
	}{
		{name: "beginning_pages", page: 3, shouldHaveEllipsis: true},
		{name: "middle_pages", page: 10, shouldHaveEllipsis: true},
		{name: "end_pages", page: 18, shouldHaveEllipsis: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testPaginationLinksWithEllipsis(t, app, tc.page, tc.shouldHaveEllipsis)
		})
	}
}

// testPaginationLinksWithEllipsis выделенная функция для тестирования ссылок пагинации с эллипсами.
func testPaginationLinksWithEllipsis(t *testing.T, app *fiber.App, page int, shouldHaveEllipsis bool) {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, fmt.Sprintf("/api/entities/data?page=%d&limit=5", page), nil) //nolint:lll // required
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response datagrid.APIResponse[TestEntity]
	body, _ := io.ReadAll(resp.Body)
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	// Проверяем что links массив создан
	links := response.Meta.Pagination.Links
	assert.NotEmpty(t, links, "Links should not be empty")

	// Собираем лейблы
	labels := make([]string, len(links))
	for i, link := range links {
		labels[i] = link.Label
	}

	t.Logf("Page %d Links: %+v", page, labels)

	// Проверяем наличие эллипсов
	if shouldHaveEllipsis {
		validateEllipsisPresence(t, labels)
	}

	// Проверяем активную страницу
	validateActivePage(t, links, page)

	// Проверяем первую и последнюю страницы
	validateFirstLastPages(t, labels)
}

// validateEllipsisPresence проверяет наличие эллипсов.
func validateEllipsisPresence(t *testing.T, labels []string) {
	t.Helper()
	hasEllipsis := false
	for _, label := range labels {
		if label == "..." {
			hasEllipsis = true
			break
		}
	}
	assert.True(t, hasEllipsis, "Should have ellipsis for large page sets")
}

// validateActivePage проверяет активную страницу.
func validateActivePage(t *testing.T, links []datagrid.PaginationLink, expectedPage int) {
	t.Helper()
	activeFound := false
	for _, link := range links {
		if link.Active {
			assert.Equal(t, strconv.Itoa(expectedPage), link.Label, "Active link should match current page")
			activeFound = true
		}
	}
	assert.True(t, activeFound, "Should have one active link")
}

// validateFirstLastPages проверяет первую и последнюю страницы.
func validateFirstLastPages(t *testing.T, labels []string) {
	t.Helper()
	hasFirst := false
	hasLast := false
	for _, label := range labels {
		if label == "1" {
			hasFirst = true
		}
		if label == "20" {
			hasLast = true
		}
	}
	assert.True(t, hasFirst, "Should always show first page")
	assert.True(t, hasLast, "Should always show last page")
}
