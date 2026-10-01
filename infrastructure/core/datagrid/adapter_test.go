package datagrid_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/infrastructure/core/datagrid"
)

// AdapterTestEntity для тестирования адаптера.
type AdapterTestEntity struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// MockRepo мок репозитория с методом GetList.
type MockRepo struct {
	data  []AdapterTestEntity
	total int
	err   error
}

func (m *MockRepo) GetTestEntities(_ context.Context, _ datagrid.Filtered) ([]AdapterTestEntity, int, error) {
	if m.err != nil {
		return nil, 0, m.err
	}
	return m.data, m.total, nil
}

func TestRepositoryAdapter_GetList(t *testing.T) {
	// Подготовка данных
	testData := []AdapterTestEntity{
		{ID: 1, Name: "Test 1"}, //nolint:goconst // required
		{ID: 2, Name: "Test 2"}, //nolint:goconst // required
	}

	mockRepo := &MockRepo{
		data:  testData,
		total: 2,
	}

	// Создаем адаптер через функцию
	adapter := datagrid.NewRepositoryAdapter[AdapterTestEntity](mockRepo.GetTestEntities)

	// Тестируем
	ctx := context.Background()
	result, total, err := adapter.GetList(ctx, datagrid.Filters{
		Limit: 10,
		Page:  1,
	})

	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, result, 2)
	assert.Equal(t, testData, result)
}

func TestRepositoryAdapter_GetList_Error(t *testing.T) {
	mockRepo := &MockRepo{
		err: assert.AnError,
	}

	adapter := datagrid.NewRepositoryAdapter[AdapterTestEntity](mockRepo.GetTestEntities)

	ctx := context.Background()
	result, total, err := adapter.GetList(ctx, datagrid.Filters{
		Limit: 10,
		Page:  1,
	})

	require.Error(t, err)
	assert.Equal(t, 0, total)
	assert.Nil(t, result)
}

func TestRepositoryAdapter_WithFilters(t *testing.T) {
	// Функция репозитория, которая учитывает фильтры
	getListFunc := func(_ context.Context, filters datagrid.Filtered) ([]AdapterTestEntity, int, error) {
		// Проверяем что фильтры передаются корректно
		if filters.GetSearch() == "test" { //nolint:goconst // required
			return []AdapterTestEntity{{ID: 1, Name: "Test 1"}}, 1, nil
		}
		return []AdapterTestEntity{}, 0, nil
	}

	adapter := datagrid.NewRepositoryAdapter[AdapterTestEntity](getListFunc)

	ctx := context.Background()

	// Тест без фильтров
	result, total, err := adapter.GetList(ctx, datagrid.Filters{
		Limit: 10,
		Page:  1,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, result)

	// Тест с фильтрами
	result, total, err = adapter.GetList(ctx, datagrid.Filters{
		Limit:  10,
		Page:   1,
		Search: "test",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, result, 1)
	assert.Equal(t, "Test 1", result[0].Name)
}

func TestNewRepositoryAdapterFromRepo(t *testing.T) {
	testData := []AdapterTestEntity{
		{ID: 1, Name: "Test 1"},
		{ID: 2, Name: "Test 2"},
	}

	mockRepo := &MockRepo{
		data:  testData,
		total: 2,
	}

	// Создаем адаптер через метод репозитория
	adapter := datagrid.NewRepositoryAdapterFromRepo[AdapterTestEntity](
		mockRepo,
		func(repo *MockRepo, ctx context.Context, filters datagrid.Filtered) ([]AdapterTestEntity, int, error) {
			return repo.GetTestEntities(ctx, filters)
		},
	)

	// Тестируем
	ctx := context.Background()
	result, total, err := adapter.GetList(ctx, datagrid.Filters{
		Limit: 10,
		Page:  1,
	})

	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, result, 2)
	assert.Equal(t, testData, result)
}

// Пример использования с реальным интерфейсом.
type UserRepository interface {
	GetUsers(ctx context.Context, filters datagrid.Filtered) ([]AdapterTestEntity, int, error)
}

type MockUserRepo struct {
	users []AdapterTestEntity
}

func (m *MockUserRepo) GetUsers(_ context.Context, _ datagrid.Filtered) ([]AdapterTestEntity, int, error) {
	return m.users, len(m.users), nil
}

func TestRepositoryAdapter_WithInterface(t *testing.T) {
	users := []AdapterTestEntity{
		{ID: 1, Name: "User 1"},
		{ID: 2, Name: "User 2"},
	}

	var userRepo UserRepository = &MockUserRepo{users: users}

	// Создаем адаптер из интерфейса
	adapter := datagrid.NewRepositoryAdapter[AdapterTestEntity](userRepo.GetUsers)

	ctx := context.Background()
	result, total, err := adapter.GetList(ctx, datagrid.Filters{
		Limit: 10,
		Page:  1,
	})

	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, result, 2)
	assert.Equal(t, users, result)
}
