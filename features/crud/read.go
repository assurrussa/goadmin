package crud

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/assurrussa/goadmin/host"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

const (
	maxReadOnlyPageSize = 100
	editAction          = "edit"
)

var (
	// ErrInvalidQuery means that a requested filter, sort or page is outside the resource's contract.
	ErrInvalidQuery = errors.New("crud: invalid list query")
	errInvalidPage  = errors.New("crud: invalid page result")
)

// The adapter authenticates the actor. Every entry point checks its actual
// resource permission, then applies the host's scoped lookups and row policy.
type reader[R, C, U any] struct {
	d        *Definition[R, C, U]
	readOnly bool
	can      func(context.Context, int64, host.PermissionDomain, host.PermissionAction) bool
}
type readDescription struct {
	Key, Title  string
	Fields      []Field
	Columns     []datagrid.Column
	MaxPageSize int
}
type readRow struct {
	ID, Revision string
	Data         map[string]any
	Actions      []string
}
type readPage struct {
	Rows  []readRow
	Total int
}

func (s reader[R, C, U]) allowed(ctx context.Context, actor host.Actor, key host.PermissionKey) bool {
	return !actor.IsZero() && integrationroles.RolePreviewAllows(ctx, key) && s.can(ctx, actor.AdminID, key.Domain, key.Action)
}

func (s reader[R, C, U]) maxPageSize() int {
	if s.readOnly {
		return maxReadOnlyPageSize
	}
	return 1000 // Existing DataGrid JSON POST limit.
}

func (s reader[R, C, U]) describe(ctx context.Context, actor host.Actor) (readDescription, error) {
	if !s.allowed(ctx, actor, s.d.Permissions.Read) {
		return readDescription{}, ErrDenied
	}
	fields := make([]Field, 0, len(s.d.Fields))
	for _, f := range s.d.Fields {
		if f.Read {
			f.Create, f.Update, f.Required = false, false, false
			f.MaxLength, f.Minimum = nil, nil
			f.Options = slices.Clone(f.Options)
			fields = append(fields, f)
		}
	}
	return readDescription{
		Key: s.d.Key, Title: s.d.Title, Fields: fields,
		Columns: cloneColumns(s.d.Columns), MaxPageSize: s.maxPageSize(),
	}, nil
}

func (s reader[R, C, U]) list(ctx context.Context, actor host.Actor, query datagrid.Filters) (readPage, error) {
	if !s.allowed(ctx, actor, s.d.Permissions.Read) {
		return readPage{}, ErrDenied
	}
	if query.Limit == 0 {
		query.Limit = s.d.PageSize
	}
	if query.Page == 0 {
		query.Page = 1
	}
	if query.SortBy == "" {
		query.SortBy = s.d.DefaultSort
	}
	if query.SortOrder == "" {
		query.SortOrder = s.d.DefaultOrder
	}
	cfg := datagrid.FilterConfig[any]{Columns: s.d.Columns, DefaultSort: s.d.DefaultSort, DefaultOrder: s.d.DefaultOrder}
	if query.Limit < 1 || query.Limit > s.maxPageSize() || query.Page < 1 || query.Page > math.MaxInt/query.Limit ||
		len(query.Validate(cfg)) != 0 || (s.readOnly && !validReadOnlyFilters(query, s.d.Columns)) {
		return readPage{}, ErrInvalidQuery
	}
	records, total, err := s.d.List(ctx, actor, query)
	if err != nil {
		return readPage{}, err
	}
	if total < 0 || len(records) > query.Limit {
		return readPage{}, errInvalidPage
	}
	rows, err := s.projectList(ctx, actor, records)
	if err != nil {
		return readPage{}, err
	}
	return readPage{Rows: rows, Total: total}, nil
}

func (s reader[R, C, U]) projectList(ctx context.Context, actor host.Actor, records []Record[R]) ([]readRow, error) {
	rows := make([]readRow, 0, len(records))
	var actions []readAction
	if len(records) > 0 {
		actions = s.permittedActions(ctx, actor)
	}
	for _, r := range records {
		if err := s.d.Authorize(ctx, actor, Read, r); err != nil {
			return nil, err
		}
		row, err := s.project(ctx, actor, r, Read, actions)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (s reader[R, C, U]) load(ctx context.Context, actor host.Actor, id string) (Record[R], error) {
	if !s.allowed(ctx, actor, s.d.Permissions.Read) {
		return Record[R]{}, ErrDenied
	}
	if !validID(id) {
		return Record[R]{}, ErrNotFound
	}
	r, err := s.d.Get(ctx, actor, id)
	if err != nil {
		return Record[R]{}, err
	}
	if id != r.ID || !s.valid(r) {
		return Record[R]{}, ErrNotFound
	}
	if err = s.d.Authorize(ctx, actor, Read, r); err != nil {
		return Record[R]{}, err
	}
	return r, nil
}

func (s reader[R, C, U]) get(ctx context.Context, actor host.Actor, id string) (readRow, error) {
	r, err := s.load(ctx, actor, id)
	if err != nil {
		return readRow{}, err
	}
	return s.project(ctx, actor, r, Read, s.permittedActions(ctx, actor))
}

func (s reader[R, C, U]) project(
	ctx context.Context, actor host.Actor, r Record[R], op Operation, permitted []readAction,
) (readRow, error) {
	if !s.valid(r) {
		return readRow{}, errors.New("crud: invalid record identity or revision")
	}
	data, err := projectFields(s.d.Fields, r, op)
	if err != nil {
		return readRow{}, err
	}
	actions := []string{}
	for _, action := range permitted {
		if s.d.Authorize(ctx, actor, action.operation, r) == nil {
			actions = append(actions, action.key)
		}
	}
	return readRow{ID: r.ID, Revision: r.Revision, Data: data, Actions: actions}, nil
}

type readAction struct {
	key       string
	operation Operation
}

// Resolve resource-wide permissions once per page; each row is still checked
// against the host policy with its unprojected data.
func (s reader[R, C, U]) permittedActions(ctx context.Context, actor host.Actor) []readAction {
	if s.readOnly || integrationroles.RolePreviewActive(ctx) {
		return nil
	}
	var allowed []readAction
	add := func(key string, permission host.PermissionKey, op Operation) {
		if s.allowed(ctx, actor, permission) {
			allowed = append(allowed, readAction{key: key, operation: op})
		}
	}
	add(editAction, s.d.Permissions.Update, Update)
	add(string(Delete), s.d.Permissions.Delete, Delete)
	for _, a := range s.d.Actions {
		add(a.Key, a.Permission, Operation(a.Key))
	}
	return allowed
}

// The read-only contract accepts only the declared sort and scalar filter values.
// Legacy CRUD keeps DataGrid's existing accepted POST inputs.
func validReadOnlyFilters(query datagrid.Filters, columns []datagrid.Column) bool {
	if !validSortOrder(query.SortOrder) {
		return false
	}
	sorted := false
	for _, c := range columns {
		if c.Key == query.SortBy && c.Sortable {
			sorted = true
		}
		v, ok := query.Fields[c.Key]
		if !ok {
			continue
		}
		if !c.Filterable || !validFilterValue(c.Type, v) {
			return false
		}
	}
	return sorted
}

func validFilterValue(kind string, value any) bool {
	switch kind {
	case fieldNumber:
		switch v := value.(type) {
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			return true
		case float64:
			return !math.IsNaN(v) && !math.IsInf(v, 0)
		case float32:
			return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0)
		}
	case fieldBoolean:
		_, ok := value.(bool)
		return ok
	case "date":
		switch d := value.(type) {
		case time.Time:
			return d.Year() >= 1 && d.Year() <= 9999
		case string:
			return validateStringValue(Field{Type: fieldDate, Required: true}, d) == nil
		}
	default:
		_, ok := value.(string)
		return ok
	}
	return false
}

func (s reader[R, C, U]) valid(r Record[R]) bool {
	if s.readOnly {
		return validID(r.ID)
	}
	return validRecord(r)
}

func validRecord[T any](r Record[T]) bool {
	return validID(r.ID) && strings.TrimSpace(r.Revision) != "" && len(r.Revision) <= 1024
}

func validID(id string) bool {
	return id != "" && id != "." && id != ".." && len(id) <= 255 &&
		strings.TrimSpace(id) == id && !strings.ContainsAny(id, ",\r\n\t\x00")
}
