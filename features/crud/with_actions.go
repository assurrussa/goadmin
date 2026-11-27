package crud

import (
	"context"

	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

// WithActionsDefinition describes actor-scoped list and detail pages with
// declared record commands. List must scope both its rows and total. Authorize
// runs for Read and for each command under its key. Command callbacks must check
// the supplied revision atomically with the write and audit.
type WithActionsDefinition[R any] struct {
	Key, BasePath, Title      string
	Description               string
	Menu                      host.Menu
	Permission                host.PermissionKey
	Fields                    []Field
	Columns                   []datagrid.Column
	ListColumns               []ListColumn
	DefaultSort, DefaultOrder string
	PageSize                  int
	ClientRequirements        []host.ClientExtensionRequirement
	List                      func(context.Context, host.Actor, datagrid.Filtered) ([]Record[R], int, error)
	Get                       func(context.Context, host.Actor, string) (Record[R], error)
	Authorize                 func(context.Context, host.Actor, Operation, Record[R]) error
	Actions                   []Action[R]
}

// NewWithActions registers list, detail, personal views and explicitly declared
// record commands. It requires revisioned records and at least one command.
func NewWithActions[R any](d WithActionsDefinition[R]) (host.Module, error) {
	return newModule(d.definition(), commandResource, nil)
}

func (d WithActionsDefinition[R]) definition() Definition[R, struct{}, struct{}] {
	return Definition[R, struct{}, struct{}]{
		Key: d.Key, BasePath: d.BasePath, Title: d.Title, Description: d.Description, Menu: d.Menu,
		Permissions: Permissions{Read: d.Permission}, Fields: d.Fields, Columns: d.Columns, ListColumns: d.ListColumns,
		DefaultSort: d.DefaultSort, DefaultOrder: d.DefaultOrder, PageSize: d.PageSize,
		ClientRequirements: d.ClientRequirements, List: d.List, Get: d.Get, Authorize: d.Authorize, Actions: d.Actions,
	}
}
