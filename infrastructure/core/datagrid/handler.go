package datagrid

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	logger "github.com/assurrussa/gologger"
	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
)

// Handler универсальный обработчик для DataGrid - МАКСИМАЛЬНО ПРОСТОЙ.
type Handler[T any] struct {
	config FilterConfig[T]
	logger logger.Logger
}

// NewHandler создает новый универсальный обработчик.
func NewHandler[T any](
	config FilterConfig[T],
	logger logger.Logger,
) *Handler[T] {
	// Устанавливаем значения по умолчанию
	if config.PageSize == 0 {
		config.PageSize = 10
	}
	if config.DefaultSort == "" {
		config.DefaultSort = "createdAt"
	}
	if config.DefaultOrder == "" {
		config.DefaultOrder = "desc"
	}
	if config.SearchMode == "" {
		config.SearchMode = SearchModeServer // по умолчанию поиск на сервере
	}
	if config.SearchPlaceholder == "" {
		config.SearchPlaceholder = "Поиск..."
	}
	if config.CreateButtonText == "" {
		config.CreateButtonText = "Создать"
	}
	if config.EmptyMessage == "" {
		config.EmptyMessage = "Нет данных для отображения"
	}
	if config.IDKey == "" {
		config.IDKey = "id"
	}
	if config.ErrorComponent == nil {
		config.ErrorComponent = func(_ fiber.Ctx, _ ErrorData, _ error) error {
			return fiber.NewError(fiber.StatusInternalServerError)
		}
	}
	if config.PageComponent == nil {
		config.PageComponent = func(c fiber.Ctx, data Response[T]) error {
			dataMap := data.ToAPIResponse()

			// Заглушка данных для проверки работоспособности, в дальнейшем может быть удален!
			bs, err := json.Marshal(map[string]any{
				"title": data.Title,
				"data":  dataMap,
				"meta":  data,
			})
			if err != nil {
				return err
			}

			return c.Send(bs)
		}
	}

	return &Handler[T]{
		config: config,
		logger: logger,
	}
}

// GetConfig возвращает текущую конфигурацию обработчика.
// Это может быть полезно для тестов или для доступа к конфигурации извне.
func (h *Handler[T]) GetConfig() FilterConfig[T] {
	return h.config
}

// parseFilters парсит фильтры из query параметров.
func (h *Handler[T]) parseFilters(c fiber.Ctx) Filters {
	return ParseFilters(c, FilterConfig[any]{
		Entity:       h.config.Entity,
		Title:        h.config.Title,
		Columns:      h.config.Columns,
		DefaultSort:  h.config.DefaultSort,
		DefaultOrder: h.config.DefaultOrder,
		PageSize:     h.config.PageSize,
	})
}

// validateFilters валидирует фильтры.
func (h *Handler[T]) validateFilters(filters Filters) []string {
	return filters.Validate(FilterConfig[any]{
		Entity:       h.config.Entity,
		Title:        h.config.Title,
		Columns:      h.config.Columns,
		DefaultSort:  h.config.DefaultSort,
		DefaultOrder: h.config.DefaultOrder,
		PageSize:     h.config.PageSize,
	})
}

// HandlePage обрабатывает GET запрос - возвращает HTML страницу с начальными данными.
func (h *Handler[T]) HandlePage(c fiber.Ctx) error {
	// Парсим фильтры из URL
	filters := h.parseFilters(c)

	// Валидируем фильтры
	if validationErrors := h.validateFilters(filters); len(validationErrors) > 0 {
		h.logger.WarnContext(c, "validation errors", logger.Error(errors.New(strings.Join(validationErrors, ", "))))
		// При ошибках валидации используем фильтры по умолчанию
		filters = Filters{
			Page:      1,
			Limit:     h.config.PageSize,
			SortBy:    h.config.DefaultSort,
			SortOrder: h.config.DefaultOrder,
			Fields:    make(map[string]any),
		}
	}

	// Получаем данные
	response, err := h.loadDataInternal(c, filters)
	if err != nil {
		h.logger.ErrorContext(c, "failed to load data", logger.Error(err))
		return h.renderErrorPage(c, err)
	}

	// Рендерим с помощью компонента
	if h.config.PageComponent != nil {
		return h.config.PageComponent(c, response)
	}

	// Если компонент не настроен - ошибка конфигурации
	return c.Status(http.StatusInternalServerError).SendString("PageComponent not configured for HTML rendering.")
}

// HandleData теперь основной метод для CSR, возвращает JSON в новом формате APIResponse.
func (h *Handler[T]) HandleData(c fiber.Ctx) error {
	var filters Filters

	// POST запросы для фильтров могут быть удобны, если фильтры сложные
	if c.Method() == http.MethodPost {
		if err := c.Bind().Body(&filters); err != nil {
			h.logger.WarnContext(c, "failed to parse body for filters, falling back to query", logger.Error(err))
			filters = h.parseFilters(c) // Fallback to query params if body parsing fails or not a JSON request
		}
	} else {
		filters = h.parseFilters(c)
	}

	// Применяем значения по умолчанию, если они не пришли в запросе (важно после парсинга body)
	if filters.Page == 0 {
		filters.Page = 1
	}
	if filters.Limit == 0 {
		filters.Limit = h.config.PageSize
	}
	if filters.SortBy == "" {
		filters.SortBy = h.config.DefaultSort
	}
	if filters.SortOrder == "" {
		filters.SortOrder = h.config.DefaultOrder
	}

	if validationErrors := h.validateFilters(filters); len(validationErrors) > 0 {
		return fiber.NewError(fiber.StatusBadRequest, strings.Join(validationErrors, ", "))
	}

	internalResponse, err := h.loadDataInternal(c, filters)
	if err != nil {
		h.logger.ErrorContext(c, "failed to load data for API", logger.Error(err))
		return fiber.NewError(fiber.StatusInternalServerError)
	}

	// Создаем API ответ используя новую структуру и конструктор
	apiResp := NewAPIResponse(internalResponse, filters.SortBy, filters.SortOrder)

	return c.JSON(apiResp) // Отправляем структурированный JSON
}

// loadDataInternal инкапсулирует логику загрузки данных, используемую и HandlePage и HandleData.
// Возвращает внутреннюю структуру Response[T].
func (h *Handler[T]) loadDataInternal(c fiber.Ctx, filters Filters) (Response[T], error) {
	data, total, err := h.config.Repository.GetList(c, filters)
	if err != nil {
		return Response[T]{}, err
	}

	if err := h.prepareRowDecorators(c, data); err != nil {
		return Response[T]{}, err
	}

	pagi := h.buildPagination(filters, total)
	filterValues := extractFilterValues(filters.Fields)
	if filters.Search != "" {
		filterValues["_search"] = filters.Search
	}
	items, err := h.buildItems(c, data)
	if err != nil {
		return Response[T]{}, err
	}

	internalResp := Response[T]{
		Items:      items,
		Pagination: pagi,

		Columns:      h.config.Columns,
		Actions:      h.config.Actions, // исходные действия (не сериализуются)
		FilterValues: filterValues,
		SortBy:       filters.SortBy,
		SortOrder:    filters.SortOrder,

		SearchMode:        h.config.SearchMode,
		SearchPlaceholder: h.config.SearchPlaceholder,
		CreateButtonText:  h.config.CreateButtonText,
		EmptyMessage:      h.config.EmptyMessage,
		Selectable:        h.config.Selectable,
		IDKey:             h.config.IDKey,
		Loading:           false,

		Creatable:   h.config.Creatable,
		Exportable:  h.config.Exportable,
		Refreshable: h.config.Refreshable,

		Entity:    h.config.Entity,
		Title:     h.config.Title,
		RoutePath: h.config.RoutePath,
	}

	if c.Method() == http.MethodGet {
		query := string(c.Request().URI().QueryString())
		internalResp.RequestQuery = &query
	}

	return internalResp, nil
}

func (h *Handler[T]) prepareRowDecorators(ctx context.Context, data []T) error {
	if len(h.config.RowDecorators) == 0 || len(data) == 0 {
		return nil
	}

	for _, decorator := range h.config.RowDecorators {
		if decorator.prepare == nil {
			continue
		}
		if err := decorator.prepare(ctx, data); err != nil {
			return fmt.Errorf("row decorator prepare: %w", err)
		}
	}

	return nil
}

func (h *Handler[T]) buildPagination(filters Filters, total int) Pagination {
	totalPages := 0
	if filters.Limit > 0 {
		totalPages = (total + filters.Limit - 1) / filters.Limit
	}
	if totalPages == 0 && total > 0 { // Если есть элементы, но лимит 0 (не должно быть, но на всякий случай)
		totalPages = 1
	}
	if total == 0 { // Если нет элементов, то и страниц 0, а не 1
		totalPages = 0
	}

	pagi := Pagination{
		CurrentPage: filters.Page,
		PerPage:     filters.Limit,
		Total:       total,
		TotalPages:  totalPages,
	}

	if totalPages == 0 {
		return pagi
	}

	from, to := CalculateFromTo(filters.Page, filters.Limit, total)
	pagi.From = from
	pagi.To = to

	if h.config.RoutePath == "" {
		return pagi
	}

	fullRoutePath := h.config.RoutePath + "/data"
	first, last, next, prev := BuildPaginationURLs(
		fullRoutePath,
		filters.Page,
		totalPages,
		filters.Limit,
		filters,
	)
	pagi.FirstPageURL = first
	pagi.LastPageURL = last
	pagi.NextPageURL = next
	pagi.PrevPageURL = prev
	pagi.Path = fullRoutePath
	pagi.Links = BuildPaginationLinks(
		fullRoutePath,
		filters.Page,
		totalPages,
		filters.Limit,
		filters,
	)

	return pagi
}

func extractFilterValues(fields map[string]any) map[string]string {
	if len(fields) == 0 {
		return map[string]string{}
	}

	result := make(map[string]string, len(fields))
	for key, value := range fields {
		if isReservedFilterKey(key) {
			continue
		}
		if encoded, ok := formatFilterValue(value); ok {
			result[key] = encoded
		}
	}

	return result
}

func (h *Handler[T]) buildItems(ctx context.Context, data []T) ([]Item[T], error) {
	itemData := make([]Item[T], 0, len(data))
	for _, item := range data {
		actions := h.availableActions(ctx, item)
		values, err := h.resolveRowValues(ctx, item)
		if err != nil {
			return nil, err
		}

		itemData = append(itemData, Item[T]{
			Item:    item,
			Actions: actions,
			Values:  values,
		})
	}

	return itemData, nil
}

func (h *Handler[T]) availableActions(ctx context.Context, item T) []Action[T] {
	if len(h.config.Actions) == 0 {
		return nil
	}

	availableActions := make([]Action[T], 0, len(h.config.Actions))
	for _, action := range h.config.Actions {
		if action.CanView == nil || action.CanView(ctx, item) {
			availableActions = append(availableActions, action)
		}
	}

	return availableActions
}

func (h *Handler[T]) resolveRowValues(ctx context.Context, item T) (map[string]any, error) {
	if len(h.config.RowDecorators) == 0 {
		return nil, nil //nolint:nilnil // it's valid
	}

	var computedValues map[string]any
	for _, decorator := range h.config.RowDecorators {
		if decorator.resolve == nil {
			continue
		}

		values, err := decorator.resolve(ctx, item)
		if err != nil {
			return nil, fmt.Errorf("row decorator resolve: %w", err)
		}
		if len(values) == 0 {
			continue
		}

		if computedValues == nil {
			computedValues = make(map[string]any, len(values))
		}
		for key, value := range values {
			computedValues[key] = value
		}
	}

	return computedValues, nil
}

// RegisterRoutes регистрирует маршруты для DataGrid.
func (h *Handler[T]) RegisterRoutes(router fiber.Router, basePath string) {
	// GET /entity - HTML страница (для CSR это будет HTML shell)
	router.Get(basePath, h.HandlePage)

	// Основной API эндпоинт для данных
	router.Post(basePath+"/data", h.HandleData) // POST для сложных фильтров в body
	router.Get(basePath+"/data", h.HandleData)  // GET для простых фильтров в query
}

type ErrorData struct {
	Status  int    `json:"status"`
	Title   string `json:"title"`
	Entity  string `json:"entity"`
	Message string `json:"message"`
}

// renderErrorPage рендерит страницу ошибки.
func (h *Handler[T]) renderErrorPage(c fiber.Ctx, err error) error {
	errorData := ErrorData{
		Status:  fiber.StatusInternalServerError,
		Title:   h.config.Title,
		Entity:  h.config.Entity,
		Message: "Произошла ошибка при загрузке данных",
	}

	// Рендерим с помощью компонента
	if h.config.ErrorComponent != nil {
		return h.config.ErrorComponent(c, errorData, err)
	}

	// Если компонент не настроен - простая ошибка
	return c.Status(http.StatusInternalServerError).SendString("Internal Server Error")
}
