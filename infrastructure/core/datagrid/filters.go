package datagrid

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
)

// ParseFilters парсит фильтры из query параметров - МАКСИМАЛЬНО ПРОСТОЙ.
func ParseFilters(c fiber.Ctx, config FilterConfig[any]) Filters {
	filters := Filters{
		Page:      1,
		Limit:     config.PageSize,
		SortBy:    config.DefaultSort,
		SortOrder: config.DefaultOrder,
		Fields:    make(map[string]any),
	}

	// Если не указан размер страницы по умолчанию, используем 10
	if filters.Limit == 0 {
		filters.Limit = 10
	}

	// Пагинация
	if page, err := strconv.Atoi(c.Query("page")); err == nil && page > 0 {
		filters.Page = page
	}
	if limit, err := strconv.Atoi(c.Query("limit")); err == nil && limit > 0 && limit <= 100 {
		filters.Limit = limit
	}

	// Поиск
	filters.Search = strings.TrimSpace(c.Query("search"))

	// Сортировка
	if sortBy := strings.TrimSpace(c.Query("sortBy")); sortBy != "" {
		if IsValidSortField(sortBy, config) {
			filters.SortBy = sortBy
		}
	}

	sortOrder := strings.ToLower(strings.TrimSpace(c.Query("sortOrder")))
	if sortOrder == sortOrderAsc || sortOrder == sortOrderDesc {
		filters.SortOrder = sortOrder
	}

	// Динамические поля из ФИЛЬТРУЕМЫХ колонок
	for _, column := range config.Columns {
		// Парсим только фильтруемые колонки
		if !column.Filterable {
			continue
		}

		value := strings.TrimSpace(c.Query(column.Key))
		if value == "" {
			continue
		}

		// Парсим значение в зависимости от типа колонки
		switch column.Type {
		case "text", "badge", "select":
			filters.Fields[column.Key] = value
		case "number":
			if num, err := strconv.Atoi(value); err == nil {
				filters.Fields[column.Key] = num
			}
		case "boolean":
			if b, err := strconv.ParseBool(value); err == nil {
				filters.Fields[column.Key] = b
			}
		case "date":
			if date, err := time.Parse("2006-01-02", value); err == nil {
				filters.Fields[column.Key] = date
			}
		default:
			// Для неизвестных типов сохраняем как строку
			filters.Fields[column.Key] = value
		}
	}

	return filters
}

// IsValidSortField проверяет, можно ли сортировать по полю.
func IsValidSortField(field string, config FilterConfig[any]) bool {
	// Базовые поля, по которым всегда можно сортировать
	baseFields := map[string]bool{
		"id":         true,
		"createdAt":  true,
		"updated_at": true,
	}

	if baseFields[field] {
		return true
	}

	// Проверяем в конфигурации колонок - только СОРТИРУЕМЫЕ
	for _, column := range config.Columns {
		if column.Key == field && column.Sortable {
			return true
		}
	}

	return false
}

// ToQueryParams преобразует фильтры в URL query параметры.
func (f Filters) ToQueryParams(config FilterConfig[any]) url.Values {
	params := url.Values{}

	// Пагинация (только если отличается от значений по умолчанию)
	if f.Page > 1 {
		params.Set("page", strconv.Itoa(f.Page))
	}
	defaultPageSize := config.PageSize
	if defaultPageSize == 0 {
		defaultPageSize = 10
	}
	if f.Limit != config.PageSize && f.Limit != defaultPageSize {
		params.Set("limit", strconv.Itoa(f.Limit))
	}

	// Поиск
	if f.Search != "" {
		params.Set("search", f.Search)
	}

	// Сортировка (только если отличается от значений по умолчанию)
	if f.SortBy != "" && f.SortBy != config.DefaultSort {
		params.Set("sortBy", f.SortBy)
	}
	if f.SortOrder != "" && f.SortOrder != config.DefaultOrder {
		params.Set("sortOrder", f.SortOrder)
	}

	// Use the same scalar representation in URLs and response metadata.
	for fieldName, value := range f.Fields {
		if isReservedFilterKey(fieldName) {
			continue
		}
		if encoded, ok := formatFilterValue(value); ok && encoded != "" {
			params.Set(fieldName, encoded)
		}
	}

	return params
}

// isReservedFilterKey protects request controls and the metadata search key from field collisions.
func isReservedFilterKey(key string) bool {
	switch key {
	case "page", "limit", "search", "sortBy", "sortOrder", "_search":
		return true
	default:
		return false
	}
}

// formatFilterValue serializes the supported filter scalars for the public string-valued contract.
func formatFilterValue(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, true
	case time.Time:
		return v.Format("2006-01-02"), true
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return fmt.Sprint(v), true
	default:
		return "", false
	}
}

// Validate валидирует фильтры - УПРОЩЕННАЯ валидация.
func (f Filters) Validate(config FilterConfig[any]) []string {
	var errors []string

	// Валидация пагинации
	if f.Page < 1 {
		errors = append(errors, "Страница должна быть больше 0")
	}
	if f.Limit < 1 || f.Limit > 1000 {
		errors = append(errors, "Лимит должен быть от 1 до 1000")
	}

	// Валидация фильтров - проверяем только фильтруемые колонки с опциями
	for _, column := range config.Columns {
		if !column.Filterable {
			continue
		}

		value, exists := f.Fields[column.Key]
		if !exists || value == nil {
			continue
		}

		// Валидация select фильтров (если есть опции)
		if err := f.validateSelectFilter(column, value); err != "" {
			errors = append(errors, err)
		}
	}

	return errors
}

// validateSelectFilter валидирует select фильтр с опциями.
func (f Filters) validateSelectFilter(column Column, value any) string {
	if len(column.FilterOptions) == 0 {
		return ""
	}

	strValue, ok := value.(string)
	if !ok {
		return ""
	}

	for _, option := range column.FilterOptions {
		if option["value"] == strValue {
			return ""
		}
	}

	return "Недопустимое значение для фильтра '" + column.Key + "'"
}

// BuildURL строит URL с текущими фильтрами.
func (f Filters) BuildURL(basePath string, config FilterConfig[any]) string {
	params := f.ToQueryParams(config)
	if len(params) == 0 {
		return basePath
	}
	return basePath + "?" + params.Encode()
}
