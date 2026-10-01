package datagrid_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/infrastructure/core/datagrid"
)

// TestUser тестовая структура пользователя.
type TestUser struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

const testNameField = "name"

func TestResponse_ToJSON_Serialization(t *testing.T) {
	// Тест что результат ToJSONRaw можно сериализовать в JSON
	response := datagrid.Response[TestUser]{
		Items: []datagrid.Item[TestUser]{
			{Item: TestUser{ID: 1, Name: "John", Email: "john@example.com"}, Actions: []datagrid.Action[TestUser]{ //nolint:goconst,lll // required
				{Key: "delete", Label: "Удалить", Icon: "fas fa-trash", Variant: "danger"}, //nolint:goconst // required
			}},
		},
		Pagination: datagrid.Pagination{
			CurrentPage: 1,
			PerPage:     10,
			Total:       1,
			TotalPages:  1,
		},
		Columns: []datagrid.Column{
			{Key: testNameField, Label: "Имя", Sortable: true},
		},
		SortBy:    testNameField,
		SortOrder: "asc", //nolint:goconst // required
	}

	jsonData := response.ToJSONWithoutError()

	// Проверяем что можно десериализовать обратно
	var unmarshaled map[string]any
	err := json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)

	// Проверяем ключевые поля
	assert.Contains(t, unmarshaled, "data")
	assert.Contains(t, unmarshaled, "meta")
	assert.Contains(t, unmarshaled, "config")
}

func TestResponse_ToJSON_Bytes_EmptyData(t *testing.T) {
	// Тест с пустыми данными
	response := datagrid.Response[TestUser]{
		Items: []datagrid.Item[TestUser]{},
		Pagination: datagrid.Pagination{
			CurrentPage: 1,
			PerPage:     10,
			Total:       0,
			TotalPages:  0,
		},
		Columns:   []datagrid.Column{},
		SortBy:    "",
		SortOrder: "asc",
	}

	jsonBytes, err := response.ToJSON()
	require.NoError(t, err)
	assert.NotEmpty(t, jsonBytes)

	var result map[string]any
	err = json.Unmarshal(jsonBytes, &result)
	require.NoError(t, err)

	// Проверяем пустые массивы
	assert.Empty(t, result["items"])
	assert.Empty(t, result["columns"])
	assert.Empty(t, result["filters"])
	assert.Empty(t, result["sortBy"])
}

func TestResponse_ToJSON_Bytes_LargeData(t *testing.T) {
	// Тест с большим количеством данных
	users := make([]datagrid.Item[TestUser], 1000)
	for i := 0; i < 1000; i++ {
		users[i] = datagrid.Item[TestUser]{
			Item: TestUser{
				ID:    i + 1,
				Name:  fmt.Sprintf("User %d", i+1),
				Email: fmt.Sprintf("user%d@example.com", i+1),
			},
			Actions: []datagrid.Action[TestUser]{},
		}
	}

	response := datagrid.Response[TestUser]{
		Items: users,
		Pagination: datagrid.Pagination{
			CurrentPage: 1,
			PerPage:     1000,
			Total:       1000,
			TotalPages:  1,
		},
		Columns: []datagrid.Column{
			{Key: "id", Label: "ID", Sortable: true},
			{Key: testNameField, Label: "Имя", Sortable: true, Filterable: true},
			{Key: "email", Label: "Email", Sortable: true}, //nolint:goconst // required
		},
	}

	jsonBytes, err := response.ToJSON()
	require.NoError(t, err)
	assert.NotEmpty(t, jsonBytes)

	// Проверяем что размер разумный (не слишком большой из-за неэффективной сериализации)
	assert.Greater(t, len(jsonBytes), 10000) // должно быть достаточно данных
	assert.Less(t, len(jsonBytes), 1000000)  // но не слишком много

	// Проверяем что можно десериализовать
	var result map[string]any
	err = json.Unmarshal(jsonBytes, &result)
	require.NoError(t, err)

	items, ok := result["data"].([]any)
	require.True(t, ok, "data должны быть []any")
	assert.Len(t, items, 1000)
}
