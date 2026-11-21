package crud

import (
	"context"
	"errors"

	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

// ReadOnlyRecord is an identifiable row with no mutation or revision contract.
type ReadOnlyRecord[R any] struct {
	ID   string `json:"id"`
	Data R      `json:"data"`
}

// ReadOnlyDefinition describes an actor-scoped list and detail page.
// List must scope both rows and total to the actor, and Get must scope the lookup.
// Authorize is a second check for each row; one refusal rejects the whole result.
// Fields use Read to expose values. Write flags and write constraints are invalid.
type ReadOnlyDefinition[R any] struct {
	Key, BasePath, Title      string
	Description               string
	Menu                      host.Menu
	Permission                host.PermissionKey
	Fields                    []Field
	Columns                   []datagrid.Column
	DefaultSort, DefaultOrder string
	PageSize                  int
	ClientRequirements        []host.ClientExtensionRequirement
	List                      func(context.Context, host.Actor, datagrid.Filtered) ([]ReadOnlyRecord[R], int, error)
	Get                       func(context.Context, host.Actor, string) (ReadOnlyRecord[R], error)
	Authorize                 func(context.Context, host.Actor, ReadOnlyRecord[R]) error
}

// NewReadOnly validates a read-only resource without I/O. The module provides
// list, detail and personal views. It registers only its read permission.
func NewReadOnly[R any](d ReadOnlyDefinition[R]) (host.Module, error) {
	return newReadOnly(d, nil)
}

// NewReadOnlyWithAdapter binds an additional transport to the same reader as
// the web resource. Build runs separately for each host runtime.
func NewReadOnlyWithAdapter[R any](d ReadOnlyDefinition[R], adapter ReadOnlyAdapter) (host.Module, error) {
	if !pathPattern.MatchString(adapter.BasePath) || adapter.Build == nil {
		return host.Module{}, errors.New("crud: read adapter requires a route namespace and builder")
	}
	return newReadOnly(d, &adapter)
}

func newReadOnly[R any](d ReadOnlyDefinition[R], adapter *ReadOnlyAdapter) (host.Module, error) {
	if d.List == nil || d.Get == nil || d.Authorize == nil {
		return host.Module{}, errors.New("crud: scoped List, Get and Authorize are required")
	}
	return newModule(d.definition(), true, adapter)
}

// Adapt the host callbacks once at construction so both declarations use the
// same permission, validation, projection and HTTP paths.
func (d ReadOnlyDefinition[R]) definition() Definition[R, struct{}, struct{}] {
	return Definition[R, struct{}, struct{}]{
		Key: d.Key, BasePath: d.BasePath, Title: d.Title, Description: d.Description, Menu: d.Menu,
		Permissions: Permissions{Read: d.Permission}, Fields: d.Fields, Columns: d.Columns,
		DefaultSort: d.DefaultSort, DefaultOrder: d.DefaultOrder, PageSize: d.PageSize,
		ClientRequirements: d.ClientRequirements,
		List: func(ctx context.Context, actor host.Actor, query datagrid.Filtered) ([]Record[R], int, error) {
			rows, total, err := d.List(ctx, actor, query)
			if err != nil {
				return nil, 0, err
			}
			if len(rows) > query.GetLimit() {
				return nil, 0, errInvalidPage
			}
			records := make([]Record[R], len(rows))
			for i, row := range rows {
				records[i] = Record[R]{ID: row.ID, Data: row.Data}
			}
			return records, total, nil
		},
		Get: func(ctx context.Context, actor host.Actor, id string) (Record[R], error) {
			row, err := d.Get(ctx, actor, id)
			return Record[R]{ID: row.ID, Data: row.Data}, err
		},
		Authorize: func(ctx context.Context, actor host.Actor, _ Operation, row Record[R]) error {
			return d.Authorize(ctx, actor, ReadOnlyRecord[R]{ID: row.ID, Data: row.Data})
		},
	}
}
