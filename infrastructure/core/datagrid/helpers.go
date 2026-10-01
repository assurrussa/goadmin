package datagrid

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/Masterminds/squirrel"
)

//go:generate toolsmocks

const (
	sortOrderAsc  = "asc"
	sortOrderDesc = "desc"

	defaultLimit = 20
)

// WhereOperator определяет тип оператора для WHERE условий.
type WhereOperator string

const (
	OpEqual        WhereOperator = "="
	OpNotEqual     WhereOperator = "<>"
	OpGreater      WhereOperator = ">"
	OpGreaterEqual WhereOperator = ">="
	OpLess         WhereOperator = "<"
	OpLessEqual    WhereOperator = "<="
	OpLike         WhereOperator = "LIKE"
	OpNotLike      WhereOperator = "NOT LIKE"
	OpIsNull       WhereOperator = "IS NULL"
	OpIsNotNull    WhereOperator = "IS NOT NULL"
	OpIn           WhereOperator = "IN"
	OpNotIn        WhereOperator = "NOT IN"

	// JSONB операторы.
	OpJSONBContains         WhereOperator = "@>"  // JSONB содержит значение
	OpJSONBContainedBy      WhereOperator = "<@"  // JSONB содержится в значении
	OpJSONBHasKey           WhereOperator = "?"   // JSONB имеет ключ
	OpJSONBHasAnyKey        WhereOperator = "?|"  // JSONB имеет любой из ключей
	OpJSONBHasAllKeys       WhereOperator = "?&"  // JSONB имеет все ключи
	OpJSONBExtractPath      WhereOperator = "#>"  // JSONB извлечь по пути (возвращает JSONB)
	OpJSONBExtractPathText  WhereOperator = "#>>" // JSONB извлечь по пути (возвращает текст)
	OpJSONBExtractField     WhereOperator = "->"  // JSONB извлечь поле (возвращает JSONB)
	OpJSONBExtractFieldText WhereOperator = "->>" // JSONB извлечь поле (возвращает текст)
	OpJSONBPathExists       WhereOperator = "@?"  // JSONB путь существует
	OpJSONBPathMatch        WhereOperator = "@@"  // JSONB путь соответствует
)

// FieldMapping определяет маппинг поля с оператором.
type FieldMapping struct {
	Column   string        // имя колонки в БД
	Operator WhereOperator // оператор сравнения
}

// NewFieldMapping создает простое маппинг с оператором равенства.
func NewFieldMapping(column string) FieldMapping {
	return FieldMapping{
		Column:   column,
		Operator: OpEqual,
	}
}

// NewFieldMappingWithOp создает маппинг с указанным оператором.
func NewFieldMappingWithOp(column string, operator WhereOperator) FieldMapping {
	return FieldMapping{
		Column:   column,
		Operator: operator,
	}
}

type SQLBuilder[T any] interface {
	OrderBy(orderBys ...string) T
	Limit(limit uint64) T
	Offset(offset uint64) T
	Where(pred any, args ...any) T
}

// SQLBuilderx помогает дополнить SQL по фильтрам.
func SQLBuilderx[T SQLBuilder[T]](sqlBuilder T, filters Filtered, allowedSortFields map[string]bool) T {
	if filters.GetLimit() > 0 {
		sqlBuilder = sqlBuilder.Limit(uint64(filters.GetLimit()))
	} else {
		sqlBuilder = sqlBuilder.Limit(defaultLimit)
	}

	if filters.GetOffset() >= 0 {
		sqlBuilder = sqlBuilder.Offset(uint64(filters.GetOffset()))
	} else {
		sqlBuilder = sqlBuilder.Offset(0)
	}

	sqlBuilder = SQLSortx(sqlBuilder, filters, allowedSortFields)

	return sqlBuilder
}

// SQLWherex помогает дополнить SQL по where с поддержкой операторов.
func SQLWherex[T SQLBuilder[T]](sqlBuilder T, filters Filtered, adoptedFields map[string]FieldMapping) T {
	if len(filters.GetFields()) == 0 {
		return sqlBuilder
	}

	for key, value := range filters.GetFields() {
		mapping, exists := adoptedFields[key]
		if !exists {
			continue // пропускаем поля которых нет в adoptedFields
		}

		// Обрабатываем разные операторы
		switch mapping.Operator {
		case OpIsNull, OpIsNotNull:
			// Для NULL операторов значение не нужно
			sqlBuilder = sqlBuilder.Where(fmt.Sprintf("%s %s", mapping.Column, mapping.Operator))

		case OpLike, OpNotLike:
			// Для LIKE добавляем % если их нет
			likeValue := fmt.Sprintf("%v", value)
			if !strings.Contains(likeValue, "%") {
				likeValue = "%" + likeValue + "%"
			}
			sqlBuilder = sqlBuilder.Where(fmt.Sprintf("%s %s ?", mapping.Column, mapping.Operator), likeValue)

		case OpIn, OpNotIn:
			// Для IN операторов ожидаем slice
			sqlBuilder = sqlBuilder.Where(fmt.Sprintf("%s %s (?)", mapping.Column, mapping.Operator), value)

		// JSONB операторы
		case OpJSONBContains, OpJSONBContainedBy:
			// @> и <@ - проверка содержания JSONB
			// Значение должно быть JSON строкой или будет преобразовано
			sqlBuilder = sqlBuilder.Where(fmt.Sprintf("%s %s ?", mapping.Column, mapping.Operator), value)

		case OpJSONBHasKey:
			// ? - проверка наличия ключа
			// Значение должно быть строкой (имя ключа)
			// Используем прямую подстановку значения в SQL
			sqlBuilder = sqlBuilder.Where(squirrel.Expr(fmt.Sprintf("%s ? '%s'", mapping.Column, value)))

		case OpJSONBHasAnyKey, OpJSONBHasAllKeys:
			// ?| и ?& - проверка наличия ключей из массива
			// Значение должно быть массивом строк
			operator := string(mapping.Operator)
			// Преобразуем массив в PostgreSQL формат
			arrayStr := formatPostgreSQLArray(value)
			sqlBuilder = sqlBuilder.Where(squirrel.Expr(fmt.Sprintf("%s %s %s", mapping.Column, operator, arrayStr)))

		case OpJSONBExtractPath, OpJSONBExtractPathText:
			// #> и #>> - извлечение по пути
			// Синтаксис: column #> '{path,to,field}' = value
			path := GetJSONBPath(key)
			operator := string(mapping.Operator)
			sqlBuilder = sqlBuilder.Where(squirrel.Expr(fmt.Sprintf("%s %s '%s' = ?", mapping.Column, operator, path), value))

		case OpJSONBExtractField, OpJSONBExtractFieldText:
			// -> и ->> - извлечение поля
			// Синтаксис: column -> 'field' = value или column ->> 'field' = value
			fieldName := GetJSONBFieldName(key)
			operator := string(mapping.Operator)
			sqlBuilder = sqlBuilder.Where(squirrel.Expr(fmt.Sprintf("%s %s '%s' = ?", mapping.Column, operator, fieldName), value))

		case OpJSONBPathExists, OpJSONBPathMatch:
			// @? и @@ - проверка пути с JSONPath
			// Значение должно быть JSONPath выражением
			operator := string(mapping.Operator)
			sqlBuilder = sqlBuilder.Where(squirrel.Expr(fmt.Sprintf("%s %s ?", mapping.Column, operator), value))

		default:
			// Для остальных операторов (=, <>, >, >=, <, <=)
			sqlBuilder = sqlBuilder.Where(fmt.Sprintf("%s %s ?", mapping.Column, mapping.Operator), value)
		}
	}

	return sqlBuilder
}

// SQLSortx помогает дополнить сортировку для SQL по фильтрам.
func SQLSortx[T SQLBuilder[T]](sqlBuilder T, filters Filtered, allowedSortFields map[string]bool) T {
	sortBy := filters.GetSortBy()
	if sortBy == "" {
		return sqlBuilder
	}

	if !allowedSortFields[sortBy] {
		return sqlBuilder
	}

	sortOrder := filters.GetSortOrder()
	switch sortOrder {
	case sortOrderAsc, sortOrderDesc:
		return sqlBuilder.OrderBy(sortBy + " " + sortOrder)
	}

	return sqlBuilder.OrderBy(sortBy)
}

// Преобразует строку вида "address.city" в PostgreSQL массив '{address,city}'.
func GetJSONBPath(fieldKey string) string {
	// Если уже в формате массива PostgreSQL, возвращаем как есть
	if strings.HasPrefix(fieldKey, "{") && strings.HasSuffix(fieldKey, "}") {
		return fieldKey
	}

	// Разбиваем по точкам и создаем массив PostgreSQL
	parts := strings.Split(fieldKey, ".")
	return "{" + strings.Join(parts, ",") + "}"
}

// Просто возвращает последнюю часть пути после последней точки.
func GetJSONBFieldName(fieldKey string) string {
	parts := strings.Split(fieldKey, ".")
	return parts[len(parts)-1]
}

// formatPostgreSQLArray преобразует Go slice в PostgreSQL array формат.
func formatPostgreSQLArray(value any) string {
	const defaultEmptyResult = "ARRAY[]"
	switch v := value.(type) {
	case []string:
		if len(v) == 0 {
			return defaultEmptyResult
		}
		// Экранируем строки и оборачиваем в кавычки
		quoted := make([]string, len(v))
		for i, s := range v {
			quoted[i] = fmt.Sprintf("'%s'", strings.ReplaceAll(s, "'", "''"))
		}
		return fmt.Sprintf("ARRAY[%s]", strings.Join(quoted, ","))
	case []int:
		if len(v) == 0 {
			return defaultEmptyResult
		}
		// Для чисел кавычки не нужны
		strValues := make([]string, len(v))
		for i, n := range v {
			strValues[i] = strconv.Itoa(n)
		}
		return fmt.Sprintf("ARRAY[%s]", strings.Join(strValues, ","))
	case []any:
		if len(v) == 0 {
			return defaultEmptyResult
		}
		// Универсальный случай
		strValues := make([]string, len(v))
		for i, item := range v {
			strValues[i] = fmt.Sprintf("'%v'", item)
		}
		return fmt.Sprintf("ARRAY[%s]", strings.Join(strValues, ","))
	default:
		// Fallback - пытаемся преобразовать в строку
		return fmt.Sprintf("'%v'", value)
	}
}

// BuildPaginationURLs создает URL'ы для пагинации как в Laravel.
func BuildPaginationURLs(
	basePath string, currentPage, totalPages, perPage int, filters Filters,
) (first, last, next, prev string) {
	if basePath == "" {
		return "", "", "", ""
	}

	// Создаем базовые параметры
	baseParams := url.Values{}
	if filters.Search != "" {
		baseParams.Set("search", filters.Search)
	}
	if filters.SortBy != "" {
		baseParams.Set("sortBy", filters.SortBy)
	}
	if filters.SortOrder != "" {
		baseParams.Set("sortOrder", filters.SortOrder)
	}
	if perPage != 10 { // default page size
		baseParams.Set("limit", strconv.Itoa(perPage))
	}

	// Добавляем динамические фильтры
	for key, value := range filters.Fields {
		if value != nil && value != "" {
			baseParams.Set(key, fmt.Sprintf("%v", value))
		}
	}

	// First page
	if totalPages > 0 {
		firstParams := baseParams
		firstParams.Set("page", "1")
		first = basePath + "?" + firstParams.Encode()
	}

	// Last page
	if totalPages > 1 {
		lastParams := baseParams
		lastParams.Set("page", strconv.Itoa(totalPages))
		last = basePath + "?" + lastParams.Encode()
	}

	// Next page
	if currentPage < totalPages {
		nextParams := baseParams
		nextParams.Set("page", strconv.Itoa(currentPage+1))
		next = basePath + "?" + nextParams.Encode()
	}

	// Previous page
	if currentPage > 1 {
		prevParams := baseParams
		prevParams.Set("page", strconv.Itoa(currentPage-1))
		prev = basePath + "?" + prevParams.Encode()
	}

	return first, last, next, prev
}

// CalculateFromTo вычисляет from и to для пагинации как в Laravel.
func CalculateFromTo(currentPage, perPage, total int) (from, to int) {
	if total == 0 {
		return 0, 0
	}

	from = (currentPage-1)*perPage + 1
	to = currentPage * perPage
	if to > total {
		to = total
	}

	return from, to
}

// BuildPaginationLinks создает массив ссылок пагинации как в Laravel.
func BuildPaginationLinks(
	basePath string, currentPage, totalPages, perPage int, filters Filters,
) []PaginationLink {
	var links []PaginationLink

	if basePath == "" || totalPages <= 1 {
		return links
	}

	// Создаем базовые параметры
	baseParams := url.Values{}
	if filters.Search != "" {
		baseParams.Set("search", filters.Search)
	}
	if filters.SortBy != "" {
		baseParams.Set("sortBy", filters.SortBy)
	}
	if filters.SortOrder != "" {
		baseParams.Set("sortOrder", filters.SortOrder)
	}
	if perPage != 10 { // default page size
		baseParams.Set("limit", strconv.Itoa(perPage))
	}

	// Добавляем динамические фильтры
	for key, value := range filters.Fields {
		if value != nil && value != "" {
			baseParams.Set(key, fmt.Sprintf("%v", value))
		}
	}

	// Функция для создания URL
	makeURL := func(page int) string {
		params := baseParams
		params.Set("page", strconv.Itoa(page))
		return basePath + "?" + params.Encode()
	}

	// Previous ссылка
	if currentPage > 1 {
		links = append(links, PaginationLink{
			URL:    makeURL(currentPage - 1),
			Label:  "Previous",
			Active: false,
		})
	} else {
		links = append(links, PaginationLink{
			URL:    "",
			Label:  "Previous",
			Active: false,
		})
	}

	// Логика генерации номеров страниц (как в Laravel)
	window := 3 // показываем по 3 страницы с каждой стороны от текущей

	start := 1
	end := totalPages

	// Если страниц много, используем логику окна
	if totalPages > 10 {
		switch {
		case currentPage <= window+2:
			// Близко к началу: 1 2 3 4 5 6 7 ... 20
			end = minInt(window*2+3, totalPages)
		case currentPage >= totalPages-window-1:
			// Близко к концу: 1 ... 14 15 16 17 18 19 20
			start = maxInt(totalPages-window*2-2, 1)
		default:
			// В середине: 1 ... 8 9 10 11 12 ... 20
			start = currentPage - window
			end = currentPage + window
		}
	}

	// Первая страница (если не входит в диапазон)
	if start > 1 {
		links = append(links, PaginationLink{
			URL:    makeURL(1),
			Label:  "1",
			Active: currentPage == 1,
		})

		if start > 2 {
			links = append(links, PaginationLink{
				URL:    "",
				Label:  "...",
				Active: false,
			})
		}
	}

	// Основной диапазон страниц
	for i := start; i <= end; i++ {
		links = append(links, PaginationLink{
			URL:    makeURL(i),
			Label:  strconv.Itoa(i),
			Active: i == currentPage,
		})
	}

	// Последняя страница (если не входит в диапазон)
	if end < totalPages {
		if end < totalPages-1 {
			links = append(links, PaginationLink{
				URL:    "",
				Label:  "...",
				Active: false,
			})
		}

		links = append(links, PaginationLink{
			URL:    makeURL(totalPages),
			Label:  strconv.Itoa(totalPages),
			Active: currentPage == totalPages,
		})
	}

	// Next ссылка
	if currentPage < totalPages {
		links = append(links, PaginationLink{
			URL:    makeURL(currentPage + 1),
			Label:  "Next",
			Active: false,
		})
	} else {
		links = append(links, PaginationLink{
			URL:    "",
			Label:  "Next",
			Active: false,
		})
	}

	return links
}

// minInt возвращает минимальное из двух чисел.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// maxInt возвращает максимальное из двух чисел.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
