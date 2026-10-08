package datagrid

import (
	"context"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
)

// Filtered интерфейс для работы с фильтрами.
type Filtered interface {
	GetPage() int
	GetOffset() int
	GetLimit() int
	GetSearch() string
	GetSortBy() string
	GetSortOrder() string
	GetFields() map[string]any
}

// Repository интерфейс для работы с любой сущностью.
type Repository[T any] interface {
	GetList(ctx context.Context, filtered Filtered) ([]T, int, error)
}

// Column структура колонки - ЕДИНОЕ описание колонки с сортировкой и фильтрацией.
type Column struct {
	Key        string `json:"key"`                  // ключ поля
	Label      string `json:"label"`                // заголовок колонки
	Type       string `json:"type,omitempty"`       // "text", "date", "number", "boolean", "badge", "select", "richtext"
	Format     string `json:"format,omitempty"`     // формат для дат
	Sortable   bool   `json:"sortable,omitempty"`   // можно ли сортировать
	Filterable bool   `json:"filterable,omitempty"` // можно ли фильтровать

	// Для фильтрации
	FilterPlaceholder string              `json:"filterPlaceholder,omitempty"` // плейсхолдер фильтра
	FilterOptions     []map[string]string `json:"filterOptions,omitempty"`     // опции для select

	// Для badge типа
	Badges map[string]map[string]string `json:"badges,omitempty"` // {"active": {"label": "Активный", "variant": "success"}}
}

// Action структура действия для Vue DataGrid.
type Action[T any] struct {
	Key     string                        `json:"key"`
	Label   string                        `json:"label"`
	Icon    string                        `json:"icon"`
	Variant string                        `json:"variant,omitempty"`
	CanView func(context.Context, T) bool `json:"-"`
}
type Item[T any] struct {
	Item    T              `json:"item"`
	Actions []Action[T]    `json:"actions"`
	Values  map[string]any `json:"values,omitempty"`
}

// PaginationLink структура ссылки пагинации (как в Laravel).
type PaginationLink struct {
	URL    string `json:"url"`    // URL страницы (null для неактивных)
	Label  string `json:"label"`  // Текст ссылки ("1", "2", "Previous", "Next", "...")
	Active bool   `json:"active"` // Активная ли страница
}

// Pagination структура пагинации для Vue DataGrid.
type Pagination struct {
	CurrentPage int `json:"currentPage"`
	PerPage     int `json:"perPage"`
	Total       int `json:"total"`
	TotalPages  int `json:"totalPages"`
	// Добавляем Laravel-like поля
	From         int              `json:"from"`
	To           int              `json:"to"`
	FirstPageURL string           `json:"firstPageUrl,omitempty"`
	LastPageURL  string           `json:"lastPageUrl,omitempty"`
	NextPageURL  string           `json:"nextPageUrl,omitempty"`
	PrevPageURL  string           `json:"prevPageUrl,omitempty"`
	Path         string           `json:"path,omitempty"`
	Links        []PaginationLink `json:"links,omitempty"` // Массив ссылок как в Laravel
}

// SearchMode режим поиска.
type SearchMode string

const (
	SearchModeServer SearchMode = "server" // поиск на сервере (по умолчанию)
	SearchModeClient SearchMode = "client" // поиск на клиенте
	SearchModeNone   SearchMode = "none"   // поиск отключен
)

// TemplateComponent функция для создания.
type TemplateComponent[T any] func(c fiber.Ctx, data Response[T]) error

// ErrorTemplateComponent функция для создания для ошибок.
type ErrorTemplateComponent func(c fiber.Ctx, data ErrorData, err error) error

// FilterConfig конфигурация для конкретной сущности - МАКСИМАЛЬНО ПРОСТАЯ.
type FilterConfig[T any] struct {
	// Основные настройки
	RoutePath  string
	Entity     string        // "users", "posts", "orders"
	Title      string        // "Пользователи", "Посты"
	Repository Repository[T] // репозиторий (не сериализуется)

	// Настройки по умолчанию
	DefaultSort  string // поле сортировки по умолчанию
	DefaultOrder string // порядок сортировки по умолчанию ("asc", "desc")
	PageSize     int    // размер страницы по умолчанию

	// ЕДИНАЯ конфигурация колонок (включает сортировку И фильтрацию)
	Columns []Column

	// Действия
	Actions []Action[T]

	// Пост-обработка строк (например, вычисление полей на основе связей)
	RowDecorators []RowDecorator[T]

	// Настройки поиска
	SearchMode        SearchMode // режим поиска: "server" или "client"
	SearchPlaceholder string     // плейсхолдер поиска

	// UI настройки
	CreateButtonText string
	EmptyMessage     string
	Selectable       bool
	IDKey            string

	// Функциональность
	Creatable   bool // показывать кнопку создания
	Exportable  bool // показывать кнопку экспорта
	Refreshable bool // показывать кнопку обновления

	// GoTempl шаблоны (опционально)
	PageComponent  TemplateComponent[T]   // Функция для создания шаблона
	ErrorComponent ErrorTemplateComponent // Функция для создания ошибки
}

type (
	RowDecoratorPrepare[T any] func(ctx context.Context, items []T) error
	RowDecoratorResolve[T any] func(ctx context.Context, item T) (map[string]any, error)
)

// RowDecorator позволяет вычислять дополнительные значения для строк DataGrid.
// Используйте prepare для батч-загрузки данных и resolve для возврата map[string]any.
type RowDecorator[T any] struct {
	prepare RowDecoratorPrepare[T]
	resolve RowDecoratorResolve[T]
}

// NewRowDecorator создаёт декоратор c шагом подготовки.
func NewRowDecorator[T any](prepare RowDecoratorPrepare[T], resolve RowDecoratorResolve[T]) RowDecorator[T] {
	return RowDecorator[T]{prepare: prepare, resolve: resolve}
}

// NewRowDecoratorResolve создаёт простой декоратор без шага подготовки.
func NewRowDecoratorResolve[T any](resolve RowDecoratorResolve[T]) RowDecorator[T] {
	return RowDecorator[T]{resolve: resolve}
}

// Filters универсальная структура фильтров (для внутреннего использования).
type Filters struct {
	// Пагинация
	Page  int `json:"page" query:"page"`
	Limit int `json:"limit" query:"limit"`

	// Поиск
	Search string `json:"search,omitempty" query:"search"`

	// Сортировка
	SortBy    string `json:"sortBy,omitempty" query:"sortBy"`
	SortOrder string `json:"sortOrder,omitempty" query:"sortOrder"`

	// Динамические фильтры (любые дополнительные поля из URL)
	Fields map[string]any `json:"fields,omitempty"`
}

// GetOffset возвращает offset для пагинации.
func (f Filters) GetOffset() int {
	return (f.Page - 1) * f.Limit
}

func (f Filters) GetPage() int {
	return f.Page
}

func (f Filters) GetLimit() int {
	return f.Limit
}

func (f Filters) GetSearch() string {
	return f.Search
}

func (f Filters) GetSortBy() string {
	return f.SortBy
}

func (f Filters) GetSortOrder() string {
	return f.SortOrder
}

func (f Filters) GetFields() map[string]any {
	return f.Fields
}

// Response ответ для Vue DataGrid - совместимый с Vue компонентом.
type Response[T any] struct {
	// RequestQuery identifies the GET query whose effective state and rows this response applies.
	// nil means provenance is unknown; an empty string means a known query without parameters.
	RequestQuery *string `json:"-"`

	// Основные данные
	Items      []Item[T]  `json:"items"`      // данные для отображения
	Pagination Pagination `json:"pagination"` // пагинация

	// Конфигурация (автоматически генерируется из Columns)
	Columns      []Column          `json:"columns"`      // колонки с настройками сортировки/фильтрации
	Actions      []Action[T]       `json:"-"`            // исходные действия с CanView функциями (не сериализуется)
	FilterValues map[string]string `json:"filterValues"` // scalar query values and _search

	// Текущее состояние
	SortBy    string `json:"sortBy"`    // текущая сортировка
	SortOrder string `json:"sortOrder"` // порядок сортировки

	// UI настройки
	SearchMode        SearchMode `json:"searchMode"`        // режим поиска
	SearchPlaceholder string     `json:"searchPlaceholder"` // плейсхолдер поиска
	CreateButtonText  string     `json:"createButtonText"`  // текст кнопки создания
	EmptyMessage      string     `json:"emptyMessage"`      // сообщение при отсутствии данных
	Selectable        bool       `json:"selectable"`        // можно ли выбирать строки
	IDKey             string     `json:"idKey"`             // ключ ID записи
	Loading           bool       `json:"loading"`           // состояние загрузки

	// Функциональность
	Creatable   bool `json:"creatable"`   // показывать кнопку создания
	Exportable  bool `json:"exportable"`  // показывать кнопку экспорта
	Refreshable bool `json:"refreshable"` // показывать кнопку обновления

	// Мета-информация
	Entity      string `json:"entityModel"` // название сущности
	Title       string `json:"title"`       // заголовок страницы
	Description string `json:"description"` // описание страницы
	RoutePath   string `json:"routePath"`
}

// ToAPIResponse преобразует Response в формат совместимый с Vue DataGrid компонентом.
func (r Response[T]) ToAPIResponse() APIResponse[T] {
	return NewAPIResponse(r, r.SortBy, r.SortOrder)
}

// ToJSON преобразует Response в формат совместимый с Vue DataGrid компонентом.
func (r Response[T]) ToJSON() ([]byte, error) {
	data := r.ToAPIResponse()

	bs, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	return bs, nil
}

// ToJSONWithoutError преобразует Response в формат совместимый с Vue DataGrid компонентом.
func (r Response[T]) ToJSONWithoutError() []byte {
	if bs, err := r.ToJSON(); err == nil {
		return bs
	}

	return nil
}

// APIResponse новый стандартный ответ API согласно ТЗ.
type APIResponse[T any] struct {
	Data   []Item[T]    `json:"data"`
	Meta   APIMeta      `json:"meta"`
	Config APIConfig[T] `json:"config"`
}

// APIMeta мета-информация для API ответа.
type APIMeta struct {
	RequestQuery *string `json:"requestQuery,omitempty"`

	Title       string            `json:"title"`
	Description string            `json:"description"`
	Pagination  APIPagination     `json:"pagination"`
	Sorting     APISorting        `json:"sorting"`
	Filters     map[string]string `json:"filters"`
}

// APIPagination информация о пагинации.
type APIPagination struct {
	CurrentPage     int   `json:"currentPage"`
	PerPage         int   `json:"perPage"`
	Total           int   `json:"total"`
	TotalPages      int   `json:"totalPages"`
	PageSizeOptions []int `json:"pageSizeOptions,omitempty"`
	// Laravel-like поля
	From         int              `json:"from"`
	To           int              `json:"to"`
	FirstPageURL string           `json:"firstPageUrl,omitempty"`
	LastPageURL  string           `json:"lastPageUrl,omitempty"`
	NextPageURL  string           `json:"nextPageUrl,omitempty"`
	PrevPageURL  string           `json:"prevPageUrl,omitempty"`
	Path         string           `json:"path,omitempty"`
	Links        []PaginationLink `json:"links,omitempty"` // Массив ссылок как в Laravel
}

// APISorting информация о сортировке.
type APISorting struct {
	SortBy    string `json:"sortBy"`
	SortOrder string `json:"sortOrder"`
}

// FrontendColumn адаптирован под фронтенд-ожидания имен полей.
type FrontendColumn struct {
	Key         string                       `json:"key"`
	Label       string                       `json:"label"`
	Type        string                       `json:"type,omitempty"`
	Format      string                       `json:"format,omitempty"`
	Sortable    bool                         `json:"sortable,omitempty"`
	Filterable  bool                         `json:"filterable,omitempty"`
	Placeholder string                       `json:"placeholder,omitempty"`
	Options     []map[string]string          `json:"options,omitempty"`
	Badges      map[string]map[string]string `json:"badges,omitempty"`
}

// APIConfig конфигурация для фронтенда.
type APIConfig[T any] struct {
	RoutePath string             `json:"routePath"`
	Columns   []FrontendColumn   `json:"columns"`
	UI        APIUIConfig        `json:"ui"`
	Behaviour APIBehaviourConfig `json:"behaviour"`
}

// APIUIConfig настройки UI.
type APIUIConfig struct {
	SearchPlaceholder string `json:"searchPlaceholder"`
	CreateButtonText  string `json:"createButtonText"`
	EmptyMessage      string `json:"emptyMessage"`
	IDKey             string `json:"idKey"`
}

// APIBehaviourConfig настройки поведения.
type APIBehaviourConfig struct {
	SearchMode  SearchMode `json:"searchMode"`
	Selectable  bool       `json:"selectable"`
	Creatable   bool       `json:"creatable"`
	Exportable  bool       `json:"exportable"`
	Refreshable bool       `json:"refreshable"`
}

// NewAPIResponse создает новый API ответ из внутреннего Response[T].
func NewAPIResponse[T any](response Response[T], sortBy, sortOrder string) APIResponse[T] {
	// Определяем опции размера страницы
	pageSizeOptions := []int{10, 25, 50, 100}

	return APIResponse[T]{
		Data: response.Items,
		Meta: APIMeta{
			RequestQuery: response.RequestQuery,
			Title:        response.Title,
			Description:  response.Description,
			Pagination: APIPagination{
				CurrentPage:     response.Pagination.CurrentPage,
				PerPage:         response.Pagination.PerPage,
				Total:           response.Pagination.Total,
				TotalPages:      response.Pagination.TotalPages,
				PageSizeOptions: pageSizeOptions,
				From:            response.Pagination.From,
				To:              response.Pagination.To,
				FirstPageURL:    response.Pagination.FirstPageURL,
				LastPageURL:     response.Pagination.LastPageURL,
				NextPageURL:     response.Pagination.NextPageURL,
				PrevPageURL:     response.Pagination.PrevPageURL,
				Path:            response.Pagination.Path,
				Links:           response.Pagination.Links,
			},
			Sorting: APISorting{
				SortBy:    sortBy,
				SortOrder: sortOrder,
			},
			Filters: response.FilterValues,
		},
		Config: APIConfig[T]{
			RoutePath: response.RoutePath,
			Columns:   convertColumns(response.Columns),
			UI: APIUIConfig{
				SearchPlaceholder: response.SearchPlaceholder,
				CreateButtonText:  response.CreateButtonText,
				EmptyMessage:      response.EmptyMessage,
				IDKey:             response.IDKey,
			},
			Behaviour: APIBehaviourConfig{
				SearchMode:  response.SearchMode,
				Selectable:  response.Selectable,
				Creatable:   response.Creatable,
				Exportable:  response.Exportable,
				Refreshable: response.Refreshable,
			},
		},
	}
}

func convertColumns(cols []Column) []FrontendColumn {
	out := make([]FrontendColumn, 0, len(cols))
	for _, c := range cols {
		out = append(out, FrontendColumn{
			Key:         c.Key,
			Label:       c.Label,
			Type:        c.Type,
			Format:      c.Format,
			Sortable:    c.Sortable,
			Filterable:  c.Filterable,
			Placeholder: c.FilterPlaceholder,
			Options:     c.FilterOptions,
			Badges:      c.Badges,
		})
	}
	return out
}

// OldAPIResponse старый стандартный ответ API (для обратной совместимости).
type OldAPIResponse[T any] struct {
	Success bool     `json:"success"`
	Data    T        `json:"data,omitempty"`
	Message string   `json:"message,omitempty"`
	Errors  []string `json:"errors,omitempty"`
}

// GetFilterableColumns возвращает только колонки, которые можно фильтровать.
func (config FilterConfig[T]) GetFilterableColumns() []Column {
	var filterable []Column
	for _, col := range config.Columns {
		if col.Filterable {
			filterable = append(filterable, col)
		}
	}
	return filterable
}

// GetSortableColumns возвращает только колонки, которые можно сортировать.
func (config FilterConfig[T]) GetSortableColumns() []Column {
	var sortable []Column
	for _, col := range config.Columns {
		if col.Sortable {
			sortable = append(sortable, col)
		}
	}
	return sortable
}
