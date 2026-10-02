package datagrid_test

import (
	"testing"

	sq "github.com/Masterminds/squirrel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/infrastructure/core/datagrid"
)

func TestSQLBuilderxFixed(t *testing.T) {
	t.Run("applies limit when provided", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			page:   1,
			limit:  15,
			fields: make(map[string]any),
		}

		allowedSortFields := map[string]bool{
			"id": true,
		}

		result := datagrid.SQLBuilderx(builder, filters, allowedSortFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "LIMIT 15")
		assert.Contains(t, sql, "OFFSET 0")
		assert.Empty(t, args)
	})

	t.Run("applies default limit when zero", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			page:   1,
			limit:  0, // нулевой лимит
			fields: make(map[string]any),
		}

		allowedSortFields := map[string]bool{
			"id": true,
		}

		result := datagrid.SQLBuilderx(builder, filters, allowedSortFields)

		sql, _, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "LIMIT 20") // defaultLimit = 20
		assert.Contains(t, sql, "OFFSET 0")
	})

	t.Run("applies offset correctly", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			page:   3,
			limit:  10,
			fields: make(map[string]any),
		}

		allowedSortFields := map[string]bool{
			"id": true,
		}

		result := datagrid.SQLBuilderx(builder, filters, allowedSortFields)

		sql, _, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "LIMIT 10")
		assert.Contains(t, sql, "OFFSET 20") // (3-1) * 10 = 20
	})

	t.Run("applies sorting when valid", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			page:      1,
			limit:     10,
			sortBy:    "username", //nolint:goconst // required
			sortOrder: "asc",      //nolint:goconst // required
			fields:    make(map[string]any),
		}

		allowedSortFields := map[string]bool{
			"username": true,
		}

		result := datagrid.SQLBuilderx(builder, filters, allowedSortFields)

		sql, _, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "ORDER BY username asc")
		assert.Contains(t, sql, "LIMIT 10")
		assert.Contains(t, sql, "OFFSET 0")
	})

	t.Run("applies where clause when fields provided", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		fields := map[string]any{
			"status":   "active",  //nolint:goconst // required
			"name":     "test",    //nolint:goconst // required
			"lastName": "testval", //nolint:goconst // required
		}

		filters := mockFilteredFixed{
			page:   1,
			limit:  10,
			fields: fields,
		}

		allowedSortFields := map[string]bool{
			"id": true,
		}

		builder = datagrid.SQLBuilderx(builder, filters, allowedSortFields)
		builder = datagrid.SQLWherex(builder, filters, map[string]datagrid.FieldMapping{
			"lastName": datagrid.NewFieldMapping("last_name"),
			"name":     datagrid.NewFieldMapping("name"),
		})

		sql, args, err := builder.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "WHERE")
		assert.Contains(t, sql, "name = $")
		assert.Contains(t, sql, "last_name = $")
		assert.Len(t, args, 2)
		assert.Contains(t, args, "test")
		assert.Contains(t, args, "testval")
	})

	t.Run("handles negative offset", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			page:   -1, // отрицательная страница
			limit:  10,
			fields: make(map[string]any),
		}

		allowedSortFields := map[string]bool{
			"id": true,
		}

		result := datagrid.SQLBuilderx(builder, filters, allowedSortFields)

		sql, _, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "LIMIT 10")
		// Отрицательный offset должен стать 0 или большим числом
		assert.Contains(t, sql, "OFFSET")
	})
}

func TestSQLSortxFixed(t *testing.T) {
	t.Run("applies sorting with valid field and asc order", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			sortBy:    "username",
			sortOrder: "asc",
		}

		allowedSortFields := map[string]bool{
			"username": true,
		}

		result := datagrid.SQLSortx(builder, filters, allowedSortFields)

		sql, _, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "ORDER BY username asc")
	})

	t.Run("applies sorting with valid field and desc order", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			sortBy:    "email", //nolint:goconst // required
			sortOrder: "desc",  //nolint:goconst // required
		}

		allowedSortFields := map[string]bool{
			"email": true,
		}

		result := datagrid.SQLSortx(builder, filters, allowedSortFields)

		sql, _, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "ORDER BY email desc")
	})

	t.Run("applies sorting with valid field but invalid order", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			sortBy:    "username",
			sortOrder: "invalid", // недопустимый порядок
		}

		allowedSortFields := map[string]bool{
			"username": true,
		}

		result := datagrid.SQLSortx(builder, filters, allowedSortFields)

		sql, _, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "ORDER BY username") // без порядка
		assert.NotContains(t, sql, "invalid")
	})

	t.Run("skips sorting when field is empty", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			sortBy:    "", // пустое поле
			sortOrder: "asc",
		}

		allowedSortFields := map[string]bool{
			"username": true,
		}

		result := datagrid.SQLSortx(builder, filters, allowedSortFields)

		sql, _, err := result.ToSql()
		require.NoError(t, err)
		assert.NotContains(t, sql, "ORDER BY")
	})

	t.Run("skips sorting when field is not allowed", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			sortBy:    "invalid_field", // недопустимое поле
			sortOrder: "asc",
		}

		allowedSortFields := map[string]bool{
			"username": true,
		}

		result := datagrid.SQLSortx(builder, filters, allowedSortFields)

		sql, _, err := result.ToSql()
		require.NoError(t, err)
		assert.NotContains(t, sql, "ORDER BY")
		assert.NotContains(t, sql, "invalid_field")
	})

	t.Run("handles empty allowed fields map", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			sortBy:    "username",
			sortOrder: "asc",
		}

		allowedSortFields := map[string]bool{} // пустая карта

		result := datagrid.SQLSortx(builder, filters, allowedSortFields)

		sql, _, err := result.ToSql()
		require.NoError(t, err)
		assert.NotContains(t, sql, "ORDER BY")
	})

	t.Run("handles nil allowed fields map", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			sortBy:    "username",
			sortOrder: "asc",
		}

		var allowedSortFields map[string]bool // nil карта

		result := datagrid.SQLSortx(builder, filters, allowedSortFields)

		sql, _, err := result.ToSql()
		require.NoError(t, err)
		assert.NotContains(t, sql, "ORDER BY")
	})

	t.Run("applies sorting with multiple allowed fields", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			sortBy:    "createdAt", //nolint:goconst // required
			sortOrder: "desc",
		}

		allowedSortFields := map[string]bool{
			"id":         true,
			"username":   true,
			"email":      true,
			"createdAt":  true,
			"updated_at": true,
		}

		result := datagrid.SQLSortx(builder, filters, allowedSortFields)

		sql, _, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "ORDER BY createdAt desc")
	})
}

func TestSQLBuilderxIntegration(t *testing.T) {
	t.Run("full integration test with all features", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "username", "email", "status").
			From("users")

		fields := map[string]any{
			"status":   "active",
			"role":     "admin", //nolint:goconst // required
			"lastName": "testval",
		}

		filters := mockFilteredFixed{
			page:      2,
			limit:     25,
			sortBy:    "username",
			sortOrder: "asc",
			fields:    fields,
		}

		allowedSortFields := map[string]bool{
			"id":       true,
			"username": true,
			"email":    true,
		}

		builder = datagrid.SQLBuilderx(builder, filters, allowedSortFields)
		builder = datagrid.SQLWherex(builder, filters, map[string]datagrid.FieldMapping{
			"lastName": datagrid.NewFieldMapping("last_name"),
			"role":     datagrid.NewFieldMapping("role"),
			"status":   datagrid.NewFieldMapping("status"),
		})

		sql, args, err := builder.ToSql()
		require.NoError(t, err)

		// Проверяем все части запроса
		assert.Contains(t, sql, "SELECT id, username, email, status FROM users")
		assert.Contains(t, sql, "WHERE")
		assert.Contains(t, sql, "status = $")
		assert.Contains(t, sql, "role = $")
		assert.Contains(t, sql, "last_name = $")
		assert.Contains(t, sql, "ORDER BY username asc")
		assert.Contains(t, sql, "LIMIT 25")
		assert.Contains(t, sql, "OFFSET 25") // (2-1) * 25 = 25

		assert.Len(t, args, 3)
		assert.Contains(t, args, "active")
		assert.Contains(t, args, "admin")
		assert.Contains(t, args, "testval")
	})

	t.Run("integration test with minimal parameters", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			page:   1,
			limit:  10,
			fields: make(map[string]any),
		}

		allowedSortFields := map[string]bool{}

		result := datagrid.SQLBuilderx(builder, filters, allowedSortFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)

		assert.Contains(t, sql, "SELECT id, name FROM users")
		assert.Contains(t, sql, "LIMIT 10")
		assert.Contains(t, sql, "OFFSET 0")
		assert.NotContains(t, sql, "WHERE")
		assert.NotContains(t, sql, "ORDER BY")
		assert.Empty(t, args)
	})
}

// Тесты для новых операторов WHERE.
func TestSQLWherexOperators(t *testing.T) {
	t.Run("equal operator", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"name": "John", //nolint:goconst // required
				"age":  25,     //nolint:goconst // required
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"name": datagrid.NewFieldMapping("user_name"),
			"age":  datagrid.NewFieldMapping("user_age"),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "user_name = $")
		assert.Contains(t, sql, "user_age = $")
		assert.Contains(t, args, "John")
		assert.Contains(t, args, 25)
	})

	t.Run("like operator", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"name":  "John",
				"email": "test@example.com",
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"name":  datagrid.NewFieldMappingWithOp("user_name", datagrid.OpLike),
			"email": datagrid.NewFieldMappingWithOp("user_email", datagrid.OpLike),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "user_name LIKE $")
		assert.Contains(t, sql, "user_email LIKE $")
		assert.Contains(t, args, "%John%")
		assert.Contains(t, args, "%test@example.com%")
	})

	t.Run("like operator with existing wildcards", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"name": "%John%",
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"name": datagrid.NewFieldMappingWithOp("user_name", datagrid.OpLike),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "user_name LIKE $")
		// Не должно добавлять дополнительные % если они уже есть
		assert.Contains(t, args, "%John%")
	})

	t.Run("not like operator", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"name": "John",
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"name": datagrid.NewFieldMappingWithOp("user_name", datagrid.OpNotLike),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "user_name NOT LIKE $")
		assert.Contains(t, args, "%John%")
	})

	t.Run("comparison operators", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"minAge": 18,
				"maxAge": 65,
				"score":  100,
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"minAge": datagrid.NewFieldMappingWithOp("age", datagrid.OpGreaterEqual),
			"maxAge": datagrid.NewFieldMappingWithOp("age", datagrid.OpLessEqual),
			"score":  datagrid.NewFieldMappingWithOp("score", datagrid.OpGreater),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "age >= $")
		assert.Contains(t, sql, "age <= $")
		assert.Contains(t, sql, "score > $")
		assert.Contains(t, args, 18)
		assert.Contains(t, args, 65)
		assert.Contains(t, args, 100)
	})

	t.Run("all comparison operators", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"equal":    "test",
				"notEqual": "test",
				"greater":  100,
				"less":     50,
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"equal":    datagrid.NewFieldMappingWithOp("field1", datagrid.OpEqual),
			"notEqual": datagrid.NewFieldMappingWithOp("field2", datagrid.OpNotEqual),
			"greater":  datagrid.NewFieldMappingWithOp("field3", datagrid.OpGreater),
			"less":     datagrid.NewFieldMappingWithOp("field4", datagrid.OpLess),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "field1 = $")
		assert.Contains(t, sql, "field2 <> $")
		assert.Contains(t, sql, "field3 > $")
		assert.Contains(t, sql, "field4 < $")
		assert.Len(t, args, 4)
	})

	t.Run("null operators", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"deletedAt":   nil,
				"lastLoginAt": "not_null",
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"deletedAt":   datagrid.NewFieldMappingWithOp("deleted_at", datagrid.OpIsNull),
			"lastLoginAt": datagrid.NewFieldMappingWithOp("last_login_at", datagrid.OpIsNotNull),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "deleted_at IS NULL")
		assert.Contains(t, sql, "last_login_at IS NOT NULL")
		// Для NULL операторов аргументы не добавляются
		assert.Empty(t, args)
	})

	t.Run("in operators", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"roles":    []string{"admin", "moderator"},
				"statuses": []int{1, 2, 3},
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"roles":    datagrid.NewFieldMappingWithOp("role", datagrid.OpIn),
			"statuses": datagrid.NewFieldMappingWithOp("status", datagrid.OpNotIn),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "role IN ($")
		assert.Contains(t, sql, "status NOT IN ($")
		assert.Contains(t, args, []string{"admin", "moderator"})
		assert.Contains(t, args, []int{1, 2, 3})
	})

	t.Run("ignores unknown fields", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"name":         "John",
				"unknownField": "should be ignored",
				"age":          25,
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"name": datagrid.NewFieldMapping("user_name"),
			"age":  datagrid.NewFieldMapping("user_age"),
			// unknownField отсутствует в adoptedFields
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		// Должно быть только 2 условия, unknownField игнорируется
		assert.Contains(t, sql, "user_name = $")
		assert.Contains(t, sql, "user_age = $")
		assert.NotContains(t, args, "should be ignored")
		assert.Len(t, args, 2)
	})

	t.Run("empty fields", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"name": datagrid.NewFieldMapping("user_name"),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.NotContains(t, sql, "WHERE")
		assert.Empty(t, args)
	})
}

// Тесты для конструкторов FieldMapping.
func TestNewFieldMapping(t *testing.T) {
	mapping := datagrid.NewFieldMapping("test_column")

	assert.Equal(t, "test_column", mapping.Column)
	assert.Equal(t, datagrid.OpEqual, mapping.Operator)
}

func TestNewFieldMappingWithOp(t *testing.T) {
	mapping := datagrid.NewFieldMappingWithOp("test_column", datagrid.OpLike)

	assert.Equal(t, "test_column", mapping.Column)
	assert.Equal(t, datagrid.OpLike, mapping.Operator)
}

// Тесты для операторов.
func TestWhereOperators(t *testing.T) {
	// Проверяем что все операторы определены правильно
	assert.Equal(t, datagrid.OpEqual, datagrid.WhereOperator("="))
	assert.Equal(t, datagrid.OpNotEqual, datagrid.WhereOperator("<>"))
	assert.Equal(t, datagrid.OpGreater, datagrid.WhereOperator(">"))
	assert.Equal(t, datagrid.OpGreaterEqual, datagrid.WhereOperator(">="))
	assert.Equal(t, datagrid.OpLess, datagrid.WhereOperator("<"))
	assert.Equal(t, datagrid.OpLessEqual, datagrid.WhereOperator("<="))
	assert.Equal(t, datagrid.OpLike, datagrid.WhereOperator("LIKE"))
	assert.Equal(t, datagrid.OpNotLike, datagrid.WhereOperator("NOT LIKE"))
	assert.Equal(t, datagrid.OpIsNull, datagrid.WhereOperator("IS NULL"))
	assert.Equal(t, datagrid.OpIsNotNull, datagrid.WhereOperator("IS NOT NULL"))
	assert.Equal(t, datagrid.OpIn, datagrid.WhereOperator("IN"))
	assert.Equal(t, datagrid.OpNotIn, datagrid.WhereOperator("NOT IN"))

	// Проверяем JSONB операторы
	assert.Equal(t, datagrid.OpJSONBContains, datagrid.WhereOperator("@>"))
	assert.Equal(t, datagrid.OpJSONBContainedBy, datagrid.WhereOperator("<@"))
	assert.Equal(t, datagrid.OpJSONBHasKey, datagrid.WhereOperator("?"))
	assert.Equal(t, datagrid.OpJSONBHasAnyKey, datagrid.WhereOperator("?|"))
	assert.Equal(t, datagrid.OpJSONBHasAllKeys, datagrid.WhereOperator("?&"))
	assert.Equal(t, datagrid.OpJSONBExtractPath, datagrid.WhereOperator("#>"))
	assert.Equal(t, datagrid.OpJSONBExtractPathText, datagrid.WhereOperator("#>>"))
	assert.Equal(t, datagrid.OpJSONBExtractField, datagrid.WhereOperator("->"))
	assert.Equal(t, datagrid.OpJSONBExtractFieldText, datagrid.WhereOperator("->>"))
	assert.Equal(t, datagrid.OpJSONBPathExists, datagrid.WhereOperator("@?"))
	assert.Equal(t, datagrid.OpJSONBPathMatch, datagrid.WhereOperator("@@"))
}

// Тесты для методов Filters.
func TestFiltersGetters(t *testing.T) {
	filters := datagrid.Filters{
		Page:      2,
		Limit:     25,
		Search:    "test search",
		SortBy:    "name",
		SortOrder: "desc",
		Fields: map[string]any{
			"status": "active",
			"role":   "admin",
		},
	}

	assert.Equal(t, 2, filters.GetPage())
	assert.Equal(t, 25, filters.GetLimit())
	assert.Equal(t, "test search", filters.GetSearch())
	assert.Equal(t, "name", filters.GetSortBy())
	assert.Equal(t, "desc", filters.GetSortOrder())
	assert.Equal(t, map[string]any{"status": "active", "role": "admin"}, filters.GetFields())
	assert.Equal(t, 25, filters.GetOffset()) // (2-1) * 25 = 25
}

func testBuilderDollar() sq.StatementBuilderType {
	return sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
}

// mockFilteredFixed реализует интерфейс datagrid.Filtered для тестов.
type mockFilteredFixed struct {
	page      int
	limit     int
	sortBy    string
	sortOrder string
	fields    map[string]any
}

func (m mockFilteredFixed) GetPage() int              { return m.page }
func (m mockFilteredFixed) GetOffset() int            { return (m.page - 1) * m.limit }
func (m mockFilteredFixed) GetLimit() int             { return m.limit }
func (m mockFilteredFixed) GetSearch() string         { return "" }
func (m mockFilteredFixed) GetSortBy() string         { return m.sortBy }
func (m mockFilteredFixed) GetSortOrder() string      { return m.sortOrder }
func (m mockFilteredFixed) GetFields() map[string]any { return m.fields }

// Тесты для JSONB операторов.
func TestSQLWherexJSONBOperators(t *testing.T) {
	const sampleEmail = "john@example.com"

	t.Run("jsonb contains operator (@>)", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"metadata": `{"country": "USA"}`,
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"metadata": datagrid.NewFieldMappingWithOp("user_metadata", datagrid.OpJSONBContains),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "user_metadata @> $")
		assert.Contains(t, args, `{"country": "USA"}`)
	})

	t.Run("jsonb contained by operator (<@)", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"subset": `{"name": "John"}`,
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"subset": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBContainedBy),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "user_data <@ $")
		assert.Contains(t, args, `{"name": "John"}`)
	})

	t.Run("jsonb has key operator (?)", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"hasEmail": "email", //nolint:goconst // required
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"hasEmail": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBHasKey),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)

		// Отладочный вывод
		t.Logf("Generated SQL: %s", sql)
		t.Logf("Args: %v", args)

		assert.Equal(t, "SELECT id, name FROM users WHERE user_data ? $1", sql)
		assert.Equal(t, []any{"email"}, args)
	})

	t.Run("jsonb has any key operator (?|)", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"hasAnyContact": []string{"email", "phone"}, //nolint:goconst // required
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"hasAnyContact": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBHasAnyKey),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "user_data")
		assert.Contains(t, sql, "?| ARRAY[$1,$2]::text[]")
		assert.Equal(t, []any{"email", "phone"}, args)
	})

	t.Run("jsonb has all keys operator (?&)", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"hasAllContact": []string{"email", "phone"},
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"hasAllContact": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBHasAllKeys),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "user_data")
		assert.Contains(t, sql, "?& ARRAY[$1,$2]::text[]")
		assert.Equal(t, []any{"email", "phone"}, args)
	})

	t.Run("jsonb extract path operator (#>)", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"address.city": "New York",
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"address.city": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBExtractPath),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "user_data #> $1::text[] = $2")
		assert.Equal(t, []any{"{address,city}", "New York"}, args)
	})

	t.Run("jsonb extract path text operator (#>>)", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"profile.name": "John Doe", //nolint:goconst // required
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"profile.name": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBExtractPathText),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "user_data #>> $1::text[] = $2")
		assert.Equal(t, []any{"{profile,name}", "John Doe"}, args)
	})

	t.Run("jsonb extract field operator (->)", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"status": `"active"`,
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"status": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBExtractField),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "user_data -> $1::text = $2")
		assert.Equal(t, []any{"status", `"active"`}, args)
	})

	t.Run("jsonb extract field text operator (->>)", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"email": sampleEmail,
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"email": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBExtractFieldText),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "user_data ->> $1::text = $2")
		assert.Equal(t, []any{"email", sampleEmail}, args)
	})

	t.Run("jsonb path exists operator (@?)", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"hasActiveStatus": `$.status ? (@ == "active")`,
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"hasActiveStatus": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBPathExists),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "user_data @? $1")
		assert.Contains(t, args, `$.status ? (@ == "active")`)
	})

	t.Run("jsonb path match operator (@@)", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"isActive": `$.status == "active"`,
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"isActive": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBPathMatch),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "user_data @@ $")
		assert.Contains(t, args, `$.status == "active"`)
	})

	t.Run("complex jsonb query with multiple operators", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"hasEmail":     "email",
				"country":      `{"address": {"country": "USA"}}`,
				"profile.name": "John",
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"hasEmail":     datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBHasKey),
			"country":      datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBContains),
			"profile.name": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBExtractPathText),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "user_data")
		assert.Contains(t, sql, "user_data @> $")
		assert.Contains(t, sql, "user_data #>> $")
		assert.Len(t, args, 4)
		assert.ElementsMatch(t, []any{"email", `{"address": {"country": "USA"}}`, "{profile,name}", "John"}, args)
	})
}

// Тесты для вспомогательных функций JSONB.
func TestJSONBHelperFunctions(t *testing.T) {
	t.Run("getJSONBPath with dot notation", func(t *testing.T) {
		result := datagrid.GetJSONBPath("address.city")
		assert.Equal(t, "{address,city}", result)
	})

	t.Run("getJSONBPath with nested path", func(t *testing.T) {
		result := datagrid.GetJSONBPath("user.profile.settings.theme")
		assert.Equal(t, "{user,profile,settings,theme}", result)
	})

	t.Run("getJSONBPath with already formatted path", func(t *testing.T) {
		result := datagrid.GetJSONBPath("{address,city}")
		assert.Equal(t, "{address,city}", result)
	})

	t.Run("getJSONBPath with single field", func(t *testing.T) {
		result := datagrid.GetJSONBPath("email")
		assert.Equal(t, "{email}", result)
	})

	t.Run("getJSONBFieldName with dot notation", func(t *testing.T) {
		result := datagrid.GetJSONBFieldName("address.city")
		assert.Equal(t, "city", result)
	})

	t.Run("getJSONBFieldName with single field", func(t *testing.T) {
		result := datagrid.GetJSONBFieldName("email")
		assert.Equal(t, "email", result)
	})

	t.Run("getJSONBFieldName with nested path", func(t *testing.T) {
		result := datagrid.GetJSONBFieldName("user.profile.settings.theme")
		assert.Equal(t, "theme", result)
	})
}

// Array binding coverage through the public SQL helper.
func TestFormatPostgreSQLArrayCoverage(t *testing.T) {
	t.Run("string array through JSONB operator", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"hasAnyKey": []string{"email", "phone", "address"}, //nolint:goconst // required
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"hasAnyKey": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBHasAnyKey),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "ARRAY[$1,$2,$3]::text[]")
		assert.Equal(t, []any{"email", "phone", "address"}, args)
	})

	t.Run("string array with quotes through JSONB operator", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"hasAnyKey": []string{"user's email", "john's phone"},
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"hasAnyKey": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBHasAnyKey),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "ARRAY[$1,$2]::text[]")
		assert.Equal(t, []any{"user's email", "john's phone"}, args)
	})

	t.Run("int array through JSONB operator", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"hasAnyKey": []int{1, 2, 3, 42},
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"hasAnyKey": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBHasAnyKey),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "ARRAY[$1,$2,$3,$4]::text[]")
		assert.Equal(t, []any{"1", "2", "3", "42"}, args)
	})

	t.Run("any array through JSONB operator", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"hasAnyKey": []any{"test", 123, "another"},
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"hasAnyKey": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBHasAnyKey),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "ARRAY[$1,$2,$3]::text[]")
		assert.Equal(t, []any{"test", "123", "another"}, args)
	})

	t.Run("non-slice fallback through JSONB operator", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"hasAnyKey": "single_value",
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"hasAnyKey": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBHasAnyKey),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "?| $1::text[]")
		assert.Equal(t, []any{"single_value"}, args)
	})

	t.Run("empty string array through JSONB operator", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"hasAnyKey": []string{},
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"hasAnyKey": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBHasAnyKey),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "ARRAY[]::text[]")
		assert.Empty(t, args)
	})

	t.Run("empty int array through JSONB operator", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"hasAnyKey": []int{},
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"hasAnyKey": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBHasAnyKey),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "ARRAY[]::text[]")
		assert.Empty(t, args)
	})

	t.Run("empty any array through JSONB operator", func(t *testing.T) {
		builder := testBuilderDollar().
			Select("id", "name").
			From("users")

		filters := mockFilteredFixed{
			fields: map[string]any{
				"hasAnyKey": []any{},
			},
		}

		adoptedFields := map[string]datagrid.FieldMapping{
			"hasAnyKey": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBHasAnyKey),
		}

		result := datagrid.SQLWherex(builder, filters, adoptedFields)

		sql, args, err := result.ToSql()
		require.NoError(t, err)
		assert.Contains(t, sql, "ARRAY[]::text[]")
		assert.Empty(t, args)
	})
}
