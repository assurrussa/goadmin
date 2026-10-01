package datagrid

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	logger "github.com/assurrussa/gologger"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func (m *MockRepository) GetList(_ context.Context, filters Filtered) ([]TestEntity, int, error) {
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

func TestHandler_MissingErrorComponent(t *testing.T) {
	mockRepo := &MockRepository{
		err: assert.AnError, // Репозиторий вернет ошибку
	}

	config := FilterConfig[TestEntity]{
		Entity:     "test",          //nolint:goconst // required
		Title:      "Test Entities", //nolint:goconst // required
		Repository: mockRepo,
		PageComponent: func(c fiber.Ctx, _ Response[TestEntity]) error {
			return c.Send([]byte("<html><body>Test Page</body></html>"))
		},
		// Не указываем ErrorComponent - должна быть простая ошибка
	}

	handler := NewHandler(config, logger.Discard())
	app := fiber.New()
	handler.RegisterRoutes(app, "/test")

	handler.config.ErrorComponent = nil

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "Internal Server Error", string(body))
}

func TestHandler_MissingComponent(t *testing.T) {
	mockRepo := &MockRepository{
		data: []TestEntity{
			{ID: 1, Name: "Test 1", Status: "active"},
		},
		total: 1,
	}

	config := FilterConfig[TestEntity]{
		Entity:     "test",
		Title:      "Test Entities",
		Repository: mockRepo,
		// Не указываем PageComponent - должна быть ошибка
		PageComponent: nil,
	}

	handler := NewHandler(config, logger.Discard())
	app := fiber.New()
	handler.RegisterRoutes(app, "/test")

	handler.config.PageComponent = nil

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "PageComponent not configured")
}

func TestHandler_RowDecorators(t *testing.T) {
	mockRepo := &MockRepository{
		data: []TestEntity{
			{ID: 1, Name: "Admin", Status: "active"},
			{ID: 2, Name: "Moderator", Status: "inactive"},
		},
		total: 2,
	}

	var prepared bool
	var resolvedCount int

	config := FilterConfig[TestEntity]{
		Entity:     "test",
		Title:      "Test Entities",
		RoutePath:  "/test",
		PageSize:   10,
		Repository: mockRepo,
		Columns: []Column{
			{Key: "id", Label: "ID", Sortable: true},
		},
		RowDecorators: []RowDecorator[TestEntity]{
			NewRowDecorator(
				func(_ context.Context, items []TestEntity) error {
					prepared = true
					require.Len(t, items, 2)
					return nil
				},
				func(_ context.Context, item TestEntity) (map[string]any, error) {
					resolvedCount++
					return map[string]any{
						"statusLabel": strings.ToUpper(item.Status),
						"title":       fmt.Sprintf("%s #%d", item.Name, item.ID),
					}, nil
				},
			),
		},
	}

	handler := NewHandler(config, logger.Discard())
	app := fiber.New()
	handler.RegisterRoutes(app, "/test")

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/test/data?limit=10&page=1", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { assert.NoError(t, resp.Body.Close()) }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var apiResp struct {
		Data []struct {
			Values map[string]any `json:"values"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &apiResp))

	assert.True(t, prepared, "decorator prepare should be called")
	assert.Equal(t, 2, resolvedCount, "resolve should be called for each item")
	require.Len(t, apiResp.Data, 2)
	assert.Equal(t, "ACTIVE", apiResp.Data[0].Values["statusLabel"])
	assert.Equal(t, "Admin #1", apiResp.Data[0].Values["title"])
}
