# JSONB Операторы в DataGrid

Этот документ описывает internal-to-goadmin helper
`goadmin/infrastructure/core/datagrid`. Он нужен встроенным admin repositories и
host-owned admin features внутри текущего Go host, но не является stable public
SDK для внешнего подключения `goadmin`. Для embedding другого проекта используйте
`goadmin/host`.

DataGrid поддерживает полный набор PostgreSQL JSONB операторов для работы с JSON данными.

## Поддерживаемые JSONB операторы

### Операторы содержания

#### `@>` - Содержит (OpJSONBContains)

Проверяет, содержит ли левый JSONB правый JSONB.

```go
adoptedFields := map[string]datagrid.FieldMapping{
    "country": datagrid.NewFieldMappingWithOp("user_metadata", datagrid.OpJSONBContains),
}

// Фильтр: {"country": `{"address": {"country": "USA"}}`}
// SQL: user_metadata @> '{"address": {"country": "USA"}}'
```

#### `<@` - Содержится в (OpJSONBContainedBy)

Проверяет, содержится ли левый JSONB в правом JSONB.

```go
adoptedFields := map[string]datagrid.FieldMapping{
    "subset": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBContainedBy),
}
```

### Операторы проверки ключей

#### `?` - Имеет ключ (OpJSONBHasKey)

Проверяет наличие ключа верхнего уровня.

```go
adoptedFields := map[string]datagrid.FieldMapping{
    "hasEmail": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBHasKey),
}

// Фильтр: {"hasEmail": "email"}
// SQL: user_data ? 'email'
```

#### `?|` - Имеет любой из ключей (OpJSONBHasAnyKey)

Проверяет наличие любого из указанных ключей.

```go
adoptedFields := map[string]datagrid.FieldMapping{
    "hasContact": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBHasAnyKey),
}

// Фильтр: {"hasContact": ["email", "phone"]}
// SQL: user_data ?| ARRAY['email','phone']
```

#### `?&` - Имеет все ключи (OpJSONBHasAllKeys)

Проверяет наличие всех указанных ключей.

```go
adoptedFields := map[string]datagrid.FieldMapping{
    "hasAllContact": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBHasAllKeys),
}

// Фильтр: {"hasAllContact": ["email", "phone"]}
// SQL: user_data ?& ARRAY['email','phone']
```

### Операторы извлечения

#### `#>` - Извлечение по пути (OpJSONBExtractPath)

Извлекает JSONB значение по указанному пути.

```go
adoptedFields := map[string]datagrid.FieldMapping{
    "address.city": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBExtractPath),
}

// Фильтр: {"address.city": "New York"}
// SQL: user_data #> '{address,city}' = 'New York'
```

#### `#>>` - Извлечение как текст (OpJSONBExtractPathText)

Извлекает значение по пути как текст.

```go
adoptedFields := map[string]datagrid.FieldMapping{
    "profile.name": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBExtractPathText),
}

// Фильтр: {"profile.name": "John"}
// SQL: user_data #>> '{profile,name}' = 'John'
```

#### `->` - Извлечение поля (OpJSONBExtractField)

Извлекает JSONB значение поля верхнего уровня.

```go
adoptedFields := map[string]datagrid.FieldMapping{
    "status": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBExtractField),
}

// Фильтр: {"status": "active"}
// SQL: user_data -> 'status' = 'active'
```

#### `->>` - Извлечение поля как текст (OpJSONBExtractFieldText)

Извлекает значение поля как текст.

```go
adoptedFields := map[string]datagrid.FieldMapping{
    "email": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBExtractFieldText),
}

// Фильтр: {"email": "user@example.com"}
// SQL: user_data ->> 'email' = 'user@example.com'
```

### JSONPath операторы

#### `@?` - Путь существует (OpJSONBPathExists)

Проверяет существование пути с использованием JSONPath.

```go
adoptedFields := map[string]datagrid.FieldMapping{
    "hasActiveStatus": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBPathExists),
}

// Фильтр: {"hasActiveStatus": `$.status ? (@ == "active")`}
// SQL: user_data @? '$.status ? (@ == "active")'
```

#### `@@` - JSONPath соответствует (OpJSONBPathMatch)

Проверяет соответствие JSONPath условию.

```go
adoptedFields := map[string]datagrid.FieldMapping{
    "matchesCondition": datagrid.NewFieldMappingWithOp("user_data", datagrid.OpJSONBPathMatch),
}

// Фильтр: {"matchesCondition": `$.age > 18`}
// SQL: user_data @@ '$.age > 18'
```

## Полный пример использования

```go
package main

import (
	"github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	"github.com/Masterminds/squirrel"
)

func ExampleJSONBFilters() {
	// Настройка маппинга полей с JSONB операторами
	adoptedFields := map[string]datagrid.FieldMapping{
		// Проверка наличия ключа
		"hasEmail": datagrid.NewFieldMappingWithOp("user_metadata", datagrid.OpJSONBHasKey),

		// Содержание JSON объекта
		"country": datagrid.NewFieldMappingWithOp("user_metadata", datagrid.OpJSONBContains),

		// Извлечение вложенного значения
		"profile.name": datagrid.NewFieldMappingWithOp("user_metadata", datagrid.OpJSONBExtractPathText),

		// Проверка наличия любого из ключей
		"hasContact": datagrid.NewFieldMappingWithOp("user_metadata", datagrid.OpJSONBHasAnyKey),

		// JSONPath условие
		"isActive": datagrid.NewFieldMappingWithOp("user_metadata", datagrid.OpJSONBPathExists),
	}

	// Создание SQL builder
	builder := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).
		Select("id", "name", "email").
		From("users")

	// Применение фильтров
	filters := mockFilters{
		fields: map[string]any{
			"hasEmail":     "email",
			"country":      `{"address": {"country": "USA"}}`,
			"profile.name": "John",
			"hasContact":   []string{"email", "phone"},
			"isActive":     `$.status ? (@ == "active")`,
		},
	}

	result := datagrid.SQLWherex(builder, filters, adoptedFields)

	sql, args, err := result.ToSql()
	// SQL будет содержать все JSONB условия
}
```

## Вспомогательные функции

### GetJSONBPath(fieldKey string) string

Преобразует точечную нотацию в PostgreSQL путь:

- `"address.city"` → `"{address,city}"`
- `"profile.settings.theme"` → `"{profile,settings,theme}"`

### GetJSONBFieldName(fieldKey string) string

Извлекает имя поля из пути:

- `"address.city"` → `"city"`
- `"profile.name"` → `"name"`

### formatPostgreSQLArray(value any) string

Преобразует Go slice в PostgreSQL ARRAY:

- `[]string{"a", "b"}` → `ARRAY['a','b']`
- `[]int{1, 2, 3}` → `ARRAY[1,2,3]`

## Примечания

1. **Безопасность**: Все значения автоматически экранируются для предотвращения SQL инъекций.

2. **Производительность**: Для JSONB полей рекомендуется создавать соответствующие индексы:

   ```sql
   -- GIN индекс для операторов @>, <@, ?, ?|, ?&
   CREATE INDEX idx_user_metadata_gin ON users USING gin (user_metadata);

   -- Индекс для конкретного пути
   CREATE INDEX idx_user_email ON users USING btree ((user_metadata->>'email'));
   ```

3. **Типы данных**:

   - Для операторов `?|` и `?&` передавайте `[]string`
   - Для операторов `@>` и `<@` передавайте JSON строки
   - Для операторов извлечения используйте точечную нотацию в ключе

4. **Совместимость**: Все JSONB операторы работают только с PostgreSQL 9.4+.
