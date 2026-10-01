# DataGrid - Простое использование

Этот документ описывает internal-to-goadmin helper
`goadmin/infrastructure/core/datagrid`. Он используется встроенными admin
handlers/repositories и host-owned admin features в текущем Go host, но не
является stable public SDK для внешнего подключения `goadmin`. Для embedding
другого проекта используйте `goadmin/host`; этот helper можно применять только
как goadmin-owned internal surface.

## Основная идея

DataGrid - это **максимально простая** и **универсальная** система для создания таблиц с данными. Одна конфигурация работает и для Go backend, и для Vue frontend.

## Ключевые принципы

1. **Максимальная простота** - одна структура `Column` описывает ВСЁ
2. **Универсальность** - работает с любыми сущностями (users, posts, orders, cities, etc.)
3. **Единая конфигурация** - настройка колонок включает сортировку И фильтрацию
4. **Всё из URL** - все фильтры, сортировка, пагинация берутся из адресной строки
5. **Унифицированная система кнопок** - единые CSS классы для всех элементов
6. **Полное покрытие тестами** - 97.7% покрытие

## Быстрый старт

### 1. Определяем сущность

```go
type User struct {
    ID        int       `json:"id"`
    Name      string    `json:"name"`
    Email     string    `json:"email"`
    Status    string    `json:"status"`
    CreatedAt time.Time `json:"created_at"`
}
```

### 2. Создаем репозиторий

```go
type UserRepository struct {
    db *sql.DB
}

func (r *UserRepository) GetList(ctx context.Context, limit, offset int, filters map[string]any) ([]User, int, error) {
    // Ваша логика получения данных с фильтрацией
    return users, total, nil
}
```

### 3. Настраиваем DataGrid

```go
config := datagrid.FilterConfig[User]{
    Entity: "users",
    Title:  "Пользователи",
    Columns: []datagrid.Column{
        {Key: "id", Label: "ID", Type: "number", Sortable: true},
        {Key: "name", Label: "Имя", Type: "text", Sortable: true, Filterable: true},
        {Key: "email", Label: "Email", Type: "text", Sortable: true, Filterable: true},
        {Key: "status", Label: "Статус", Type: "select", Sortable: true, Filterable: true,
            FilterOptions: []map[string]string{
                {"value": "active", "label": "Активный"},
                {"value": "inactive", "label": "Неактивный"},
            },
            Badges: map[string]map[string]string{
                "active": {"label": "Активный", "variant": "success"},
                "inactive": {"label": "Неактивный", "variant": "danger"},
            },
        },
        {Key: "created_at", Label: "Создан", Type: "date", Sortable: true},
    },
    Actions: []datagrid.Action{
        {Key: "edit", Label: "Редактировать", Icon: "edit", Variant: "primary"},
        {Key: "delete", Label: "Удалить", Icon: "trash", Variant: "danger"},
    },
    Behaviour: datagrid.Behaviour{
        Searchable: true,
        Creatable: true,
        InlineCreatable: true,  // 🆕 Новая возможность
        Selectable: true,
    },
    UI: datagrid.UI{
        Title: "Управление пользователями",
        Description: "Просмотр и управление пользователями системы",
        CreateButtonText: "Добавить пользователя",
        EmptyMessage: "Пользователи не найдены",
    },
    Repository: userRepo,
}
```

## 🆕 Новые возможности

### Inline создание записей

Теперь можно добавлять записи прямо в таблице:

```go
Behaviour: datagrid.Behaviour{
    Creatable: true,        // Кнопка "Создать" в заголовке
    InlineCreatable: true,  // Кнопка "Добавить запись" в таблице
}
```

**Визуальный результат:**

- Зеленая строка в таблице с кнопкой "Добавить запись"
- Появляется только когда есть данные в таблице
- Плавные hover эффекты
- Единообразные стили

### Унифицированная система кнопок

Все кнопки теперь используют единую CSS систему:

```vue
<!-- Кнопки действий в таблице -->
<button class="btn btn-primary btn-xs">Редактировать</button>
<button class="btn btn-danger btn-xs">Удалить</button>

<!-- Кнопка создания -->
<button class="btn btn-primary">Создать</button>

<!-- Пагинация -->
<button class="btn btn-secondary btn-sm">1</button>
<button class="btn btn-primary btn-sm">2</button>
```

### Улучшенные размеры кнопок

- `btn-xs` - для действий в таблице
- `btn-sm` - для пагинации и компактных элементов
- `btn-md` - стандартный размер
- `btn-lg` - для важных действий
- `btn-xl` - для hero секций

- ### RowDecorators — вычисляемые значения строк

Теперь можно обогащать строки данными, которых нет напрямую в сущности (например, роли или агрегированные поля из связей):

```go
config := datagrid.FilterConfig[models.Admin]{
    // ...
    RowDecorators: []datagrid.RowDecorator[models.Admin]{
        datagrid.NewRowDecoratorResolve(func(ctx context.Context, admin models.Admin) (map[string]any, error) {
            return map[string]any{
                "role":        admin.GetRole(),
                "permissions": flattenPermissions(admin.GetPermissions()),
            }, nil
        }),
    },
}
```

На фронтенде данные доступны через `item.values.role`. DataGrid автоматически объединяет несколько декораторов и вызывает `Prepare` один раз на батч, что позволяет загружать связи оптом перед генерацией строк.

## Интеграция с Vue

Vue компонент автоматически получает всю необходимую конфигурацию:

```vue
<template>
  <DataGrid
    :api-url="apiUrl"
    :initial-data="apiResponseData"
    @create="handleCreate"
    @action="handleAction"
    @page-change="handlePageChange"
  />
</template>
```

**Автоматические возможности:**

- ✅ Кнопка создания (если `Creatable: true`)
- ✅ Inline создание (если `InlineCreatable: true`)
- ✅ Поиск (если `Searchable: true`)
- ✅ Фильтрация (по колонкам с `Filterable: true`)
- ✅ Сортировка (по колонкам с `Sortable: true`)
- ✅ Выбор записей (если `Selectable: true`)
- ✅ Пагинация (автоматически)
- ✅ Пустое состояние с кнопкой создания

## Примеры конфигураций

### Простая таблица только для чтения

```go
config := datagrid.FilterConfig[City]{
    Entity: "cities",
    Title:  "Города",
    Columns: []datagrid.Column{
        {Key: "id", Label: "ID", Type: "number"},
        {Key: "name", Label: "Название", Type: "text"},
        {Key: "population", Label: "Население", Type: "number"},
    },
    Repository: cityRepo,
}
```

### Полнофункциональная таблица

```go
config := datagrid.FilterConfig[User]{
    Entity: "users",
    Title:  "Пользователи",
    Columns: []datagrid.Column{
        {Key: "id", Label: "ID", Type: "number", Sortable: true},
        {Key: "name", Label: "Имя", Type: "text", Sortable: true, Filterable: true},
        {Key: "status", Label: "Статус", Type: "select", Sortable: true, Filterable: true,
            FilterOptions: []map[string]string{
                {"value": "active", "label": "Активный"},
                {"value": "inactive", "label": "Неактивный"},
            }},
    },
    Actions: []datagrid.Action{
        {Key: "edit", Label: "Редактировать", Variant: "primary"},
        {Key: "delete", Label: "Удалить", Variant: "danger"},
    },
    Behaviour: datagrid.Behaviour{
        Searchable: true,
        Creatable: true,
        InlineCreatable: true,
        Selectable: true,
    },
    Repository: userRepo,
}
```

### Компактная таблица с inline созданием

```go
config := datagrid.FilterConfig[Tag]{
    Entity: "tags",
    Title:  "Теги",
    Columns: []datagrid.Column{
        {Key: "name", Label: "Название", Type: "text", Filterable: true},
        {Key: "color", Label: "Цвет", Type: "badge"},
    },
    Behaviour: datagrid.Behaviour{
        InlineCreatable: true,  // Только inline создание
    },
    Repository: tagRepo,
}
```

## CSS классы кнопок

### Варианты

- `btn-primary` - основные действия (создать, сохранить)
- `btn-secondary` - вторичные действия (отмена, назад)
- `btn-danger` - опасные действия (удалить)
- `btn-success` - успешные действия (активировать)
- `btn-warning` - предупреждения
- `btn-ghost` - прозрачные кнопки
- `btn-outline` - кнопки с границей

### Размеры

- `btn-xs` - 0.25rem 0.5rem, font-size: 0.75rem
- `btn-sm` - 0.375rem 0.75rem, font-size: 0.875rem
- `btn-md` - 0.5rem 1rem, font-size: 0.875rem (по умолчанию)
- `btn-lg` - 0.625rem 1.25rem, font-size: 1rem
- `btn-xl` - 0.75rem 1.5rem, font-size: 1.125rem

### Модификаторы

- `btn-full` - на всю ширину
- `btn-rounded` - круглые углы
- `btn-loading` - с анимацией загрузки

## Заключение

DataGrid обеспечивает:

- ✅ **Максимальную простоту** - одна конфигурация для всего
- ✅ **Универсальность** - работает с любыми сущностями
- ✅ **Полную интеграцию** Go ↔ Vue без преобразований
- ✅ **Унифицированные стили** - единая система кнопок
- ✅ **Гибкость создания** - обычное и inline создание
- ✅ **Современный UX** - плавные анимации и hover эффекты
- ✅ **Легкость поддержки** - CSS переменные и централизованные стили

В текущем admin runtime это рабочий internal helper. Перед тем как делать его
частью stable public SDK, нужно отдельно зафиксировать контракт пакета,
совместимость Vue props и external-consumer verification.

## Типы колонок

### text - Обычный текст

```go
{Key: "name", Label: "Имя", Type: "text", Sortable: true, Filterable: true}
```

### number - Числа

```go
{Key: "age", Label: "Возраст", Type: "number", Sortable: true, Filterable: true}
```

### date - Даты

```go
{Key: "created_at", Label: "Создан", Type: "date", Format: "dd.MM.yyyy", Sortable: true}
```

### boolean - Да/Нет

```go
{Key: "active", Label: "Активен", Type: "boolean", Sortable: false, Filterable: true}
```

### select - Выбор из списка

```go
{
    Key: "status",
    Label: "Статус",
    Type: "select",
    Sortable: true,
    Filterable: true,
    FilterOptions: []map[string]string{
        {"value": "active", "label": "Активный"},
        {"value": "inactive", "label": "Неактивный"},
    },
    Badges: map[string]map[string]string{
        "active": {"label": "Активный", "variant": "success"},
        "inactive": {"label": "Неактивный", "variant": "danger"},
    },
}
```

## Режимы поиска

### Серверный поиск (по умолчанию)

```go
SearchMode: datagrid.SearchModeServer
```

- Поиск выполняется на сервере
- Подходит для больших объемов данных
- Требует реализации поиска в репозитории

### Клиентский поиск

```go
SearchMode: datagrid.SearchModeClient
```

- Поиск выполняется на клиенте (Vue)
- Подходит для небольших объемов данных (до 100-200 записей)
- Не требует дополнительных запросов к серверу

## Примеры для разных сущностей

### Посты блога

```go
config := datagrid.FilterConfig[Post]{
    Entity: "posts",
    Title:  "Посты",
    Columns: []datagrid.Column{
        {Key: "id", Label: "ID", Type: "number", Sortable: true},
        {Key: "title", Label: "Заголовок", Type: "text", Sortable: true, Filterable: true},
        {Key: "category", Label: "Категория", Type: "select", Sortable: true, Filterable: true,
            FilterOptions: []map[string]string{
                {"value": "tech", "label": "Технологии"},
                {"value": "news", "label": "Новости"},
            }},
        {Key: "published", Label: "Опубликован", Type: "boolean", Sortable: true, Filterable: true},
        {Key: "created_at", Label: "Создан", Type: "date", Sortable: true},
    },
    Repository: postRepo,
}
```

### Заказы

```go
config := datagrid.FilterConfig[Order]{
    Entity: "orders",
    Title:  "Заказы",
    Columns: []datagrid.Column{
        {Key: "id", Label: "№", Type: "number", Sortable: true},
        {Key: "customer_name", Label: "Клиент", Type: "text", Sortable: true, Filterable: true},
        {Key: "status", Label: "Статус", Type: "select", Sortable: true, Filterable: true,
            FilterOptions: []map[string]string{
                {"value": "new", "label": "Новый"},
                {"value": "processing", "label": "В обработке"},
                {"value": "completed", "label": "Завершен"},
                {"value": "cancelled", "label": "Отменен"},
            }},
        {Key: "total", Label: "Сумма", Type: "number", Sortable: true},
        {Key: "created_at", Label: "Создан", Type: "date", Sortable: true},
    },
    Repository: orderRepo,
}
```

### Города

```go
config := datagrid.FilterConfig[City]{
    Entity: "cities",
    Title:  "Города",
    Columns: []datagrid.Column{
        {Key: "id", Label: "ID", Type: "number", Sortable: true},
        {Key: "name", Label: "Название", Type: "text", Sortable: true, Filterable: true},
        {Key: "region", Label: "Регион", Type: "select", Sortable: true, Filterable: true,
            FilterOptions: []map[string]string{
                {"value": "moscow", "label": "Московская область"},
                {"value": "spb", "label": "Ленинградская область"},
            }},
        {Key: "population", Label: "Население", Type: "number", Sortable: true},
    },
    Repository: cityRepo,
}
```

## Методы для Vue.js интеграции

### ToJSON()

Метод `ToJSON()` преобразует `Response` в JSON байты, совместимые с Vue DataGrid компонентом:

```go
// Создаем Response
response := datagrid.Response[User]{
    Items: users,
    Pagination: datagrid.Pagination{
        CurrentPage: 1,
        PerPage:     10,
        Total:       100,
        TotalPages:  10,
    },
    Columns: []datagrid.Column{
        {Key: "name", Label: "Имя", Sortable: true, Filterable: true},
        {Key: "email", Label: "Email", Sortable: true},
    },
    SortBy:    "name",
    SortOrder: "asc",
}

// Преобразуем в JSON байты
jsonBytes, err := response.ToJSON()
if err != nil {
    log.Fatal(err)
}

// Можно сразу отправить клиенту
return c.Send(jsonBytes)
```

### ToJSONRaw()

Метод `ToJSONRaw()` возвращает сырые данные в виде `map[string]any` без JSON сериализации:

```go
// Получаем сырые данные
vueData := response.ToJSONRaw()

// Результат содержит:
// {
//   "items": [...],
//   "pagination": {
//     "current_page": 1,
//     "per_page": 10,
//     "total": 100,
//     "total_pages": 10
//   },
//   "columns": [...],
//   "filters": [...], // только фильтруемые колонки
//   "sortBy": "name",
//   "sortOrder": "asc",
//   ...
// }

// Можно дополнительно обработать данные
vueData["custom_field"] = "custom_value"

// И потом сериализовать
jsonBytes, _ := json.Marshal(vueData)
```

### ToAPIResponse()

Метод `ToAPIResponse()` упаковывает Response в стандартный API ответ:

```go
// Создаем Response
response := datagrid.Response[User]{...}

// Упаковываем в API ответ
apiResponse := response.ToAPIResponse()

// Результат:
// {
//   "success": true,
//   "data": {...}, // результат ToJSONRaw()
//   "message": "",
//   "errors": []
// }
```

### Использование в Fiber контроллере

```go
func (h *UserHandler) GetUsers(c fiber.Ctx) error {
    // Получаем данные через datagrid
    response, err := h.datagridHandler.LoadData(c)
    if err != nil {
        return c.Status(500).JSON(fiber.Map{
            "success": false,
            "message": "Ошибка загрузки данных",
        })
    }

    // Вариант 1: Возвращаем готовые JSON байты
    jsonBytes, err := response.ToJSON()
    if err != nil {
        return c.Status(500).JSON(fiber.Map{"error": "JSON serialization failed"})
    }
    c.Set("Content-Type", "application/json")
    return c.Send(jsonBytes)

    // Вариант 2: Возвращаем через Fiber JSON (автоматическая сериализация)
    return c.JSON(response.ToAPIResponse())

    // Вариант 3: Кастомная обработка данных
    vueData := response.ToJSONRaw()
    vueData["server_time"] = time.Now().Unix()
    return c.JSON(fiber.Map{"success": true, "data": vueData})
}
```

### Использование во Vue компоненте

```javascript
// Загружаем данные с сервера
const response = await fetch("/api/users");
const apiResponse = await response.json();

if (apiResponse.success) {
  const vueData = apiResponse.data;

  // Передаем данные в DataGrid компонент
  this.items = vueData.items;
  this.columns = vueData.columns;
  this.pagination = vueData.pagination;
  this.filters = vueData.filters;
  this.sortBy = vueData.sortBy;
  this.sortOrder = vueData.sortOrder;
  // ... остальные поля
}
```

### Особенности преобразования

1. **Пагинация**: Поля `CurrentPage`, `PerPage`, `Total`, `TotalPages` преобразуются в `current_page`, `per_page`, `total`, `total_pages` (snake_case для Vue)

2. **Фильтры**: Автоматически извлекаются только фильтруемые колонки (`Filterable: true`) и преобразуются в формат Vue:

   ```go
   // Go Column
   {
       Key: "status",
       Filterable: true,
       FilterPlaceholder: "Выберите статус",
       FilterOptions: []map[string]string{
           {"value": "active", "label": "Активный"},
           {"value": "inactive", "label": "Неактивный"},
       },
   }

   // Vue filter
   {
       "key": "status",
       "placeholder": "Выберите статус",
       "options": [
           {"value": "active", "label": "Активный"},
           {"value": "inactive", "label": "Неактивный"}
       ]
   }
   ```

3. **JSON сериализация**: Результат `ToJSON()` полностью совместим с JSON и может быть безопасно сериализован/десериализован.
