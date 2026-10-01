package datagrid

import (
	logger "github.com/assurrussa/gologger"
	"github.com/gofiber/fiber/v3"

	core "github.com/assurrussa/goadmin/infrastructure/core/datagrid"
)

type (
	Filtered                   = core.Filtered
	Repository[T any]          = core.Repository[T]
	Column                     = core.Column
	Action[T any]              = core.Action[T]
	Item[T any]                = core.Item[T]
	PaginationLink             = core.PaginationLink
	Pagination                 = core.Pagination
	SearchMode                 = core.SearchMode
	TemplateComponent[T any]   = core.TemplateComponent[T]
	ErrorTemplateComponent     = core.ErrorTemplateComponent
	FilterConfig[T any]        = core.FilterConfig[T]
	RowDecoratorPrepare[T any] = core.RowDecoratorPrepare[T]
	RowDecoratorResolve[T any] = core.RowDecoratorResolve[T]
	RowDecorator[T any]        = core.RowDecorator[T]
	Filters                    = core.Filters
	Response[T any]            = core.Response[T]
	APIResponse[T any]         = core.APIResponse[T]
	WhereOperator              = core.WhereOperator
	FieldMapping               = core.FieldMapping
	Handler[T any]             = core.Handler[T]
)

const (
	SearchModeServer = core.SearchModeServer
	SearchModeClient = core.SearchModeClient
	SearchModeNone   = core.SearchModeNone

	OpEqual                 = core.OpEqual
	OpNotEqual              = core.OpNotEqual
	OpGreater               = core.OpGreater
	OpGreaterEqual          = core.OpGreaterEqual
	OpLess                  = core.OpLess
	OpLessEqual             = core.OpLessEqual
	OpLike                  = core.OpLike
	OpNotLike               = core.OpNotLike
	OpIsNull                = core.OpIsNull
	OpIsNotNull             = core.OpIsNotNull
	OpIn                    = core.OpIn
	OpNotIn                 = core.OpNotIn
	OpJSONBContains         = core.OpJSONBContains
	OpJSONBContainedBy      = core.OpJSONBContainedBy
	OpJSONBHasKey           = core.OpJSONBHasKey
	OpJSONBHasAnyKey        = core.OpJSONBHasAnyKey
	OpJSONBHasAllKeys       = core.OpJSONBHasAllKeys
	OpJSONBExtractPath      = core.OpJSONBExtractPath
	OpJSONBExtractPathText  = core.OpJSONBExtractPathText
	OpJSONBExtractField     = core.OpJSONBExtractField
	OpJSONBExtractFieldText = core.OpJSONBExtractFieldText
	OpJSONBPathExists       = core.OpJSONBPathExists
	OpJSONBPathMatch        = core.OpJSONBPathMatch
)

func NewHandler[T any](config FilterConfig[T], lg logger.Logger) *Handler[T] {
	return core.NewHandler(config, lg)
}

func NewRowDecorator[T any](prepare RowDecoratorPrepare[T], resolve RowDecoratorResolve[T]) RowDecorator[T] {
	return core.NewRowDecorator(prepare, resolve)
}

func NewRowDecoratorResolve[T any](resolve RowDecoratorResolve[T]) RowDecorator[T] {
	return core.NewRowDecoratorResolve(resolve)
}

func NewFieldMapping(column string) FieldMapping {
	return core.NewFieldMapping(column)
}

func NewFieldMappingWithOp(column string, operator WhereOperator) FieldMapping {
	return core.NewFieldMappingWithOp(column, operator)
}

func SQLBuilderx[T core.SQLBuilder[T]](sqlBuilder T, filters Filtered, allowedSortFields map[string]bool) T {
	return core.SQLBuilderx(sqlBuilder, filters, allowedSortFields)
}

func SQLWherex[T core.SQLBuilder[T]](sqlBuilder T, filters Filtered, adoptedFields map[string]FieldMapping) T {
	return core.SQLWherex(sqlBuilder, filters, adoptedFields)
}

func SQLSortx[T core.SQLBuilder[T]](sqlBuilder T, filters Filtered, allowedSortFields map[string]bool) T {
	return core.SQLSortx(sqlBuilder, filters, allowedSortFields)
}

func ParseFilters(c fiber.Ctx, config FilterConfig[any]) Filters {
	return core.ParseFilters(c, config)
}

func IsValidSortField(field string, config FilterConfig[any]) bool {
	return core.IsValidSortField(field, config)
}

func GetJSONBPath(fieldKey string) string {
	return core.GetJSONBPath(fieldKey)
}

func GetJSONBFieldName(fieldKey string) string {
	return core.GetJSONBFieldName(fieldKey)
}

func BuildPaginationURLs(basePath string, currentPage, totalPages, perPage int, filters Filters) (string, string, string, string) { //nolint:revive,lll // required
	return core.BuildPaginationURLs(basePath, currentPage, totalPages, perPage, filters)
}
