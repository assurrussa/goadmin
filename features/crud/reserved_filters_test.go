//nolint:goconst // Explicit field names and kinds keep query-control collision fixtures readable.
package crud //nolint:testpackage // Exercises constructor validation beside shared fixtures.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

type queryControlRow struct {
	Page           string `json:"page"`
	Limit          string `json:"limit"`
	Search         string `json:"search"`
	SortBy         string `json:"sortBy"`
	SortOrder      string `json:"sortOrder"`
	Scope          string `json:"scope"`
	Columns        string `json:"columns"`
	IDs            string `json:"ids"`
	Name           string `json:"name"`
	ToString       string `json:"toString"`
	ValueOf        string `json:"valueOf"`
	HasOwnProperty string `json:"hasOwnProperty"`
}

func queryControlDefinition() Definition[queryControlRow, queryControlRow, queryControlRow] {
	base := definition()
	return Definition[queryControlRow, queryControlRow, queryControlRow]{
		Key: base.Key, BasePath: base.BasePath, Title: base.Title, Permissions: base.Permissions,
		List: func(context.Context, host.Actor, datagrid.Filtered) ([]Record[queryControlRow], int, error) {
			return nil, 0, nil
		},
		Get: func(context.Context, host.Actor, string) (Record[queryControlRow], error) {
			return Record[queryControlRow]{}, nil
		},
		LoadSelected: func(context.Context, host.Actor, []string) ([]Record[queryControlRow], error) {
			return nil, nil
		},
		Create: func(context.Context, host.Actor, queryControlRow) (Record[queryControlRow], error) {
			return Record[queryControlRow]{}, nil
		},
		Update: func(context.Context, host.Actor, string, string, queryControlRow) (Record[queryControlRow], error) {
			return Record[queryControlRow]{}, nil
		},
		Delete: base.Delete,
		Authorize: func(context.Context, host.Actor, Operation, Record[queryControlRow]) error {
			return nil
		},
	}
}

func TestConstructorRejectsOnlyFilterableQueryControlColumns(t *testing.T) {
	d := queryControlDefinition()
	for _, key := range []string{
		"page", "limit", "search", "sortBy", "sortOrder", "scope", "columns", "ids", "name", "toString", "valueOf",
	} {
		t.Run(key, func(t *testing.T) {
			d.Fields = []Field{{Key: key, Label: key, Type: "text", Read: true, Create: true, Update: true}}
			d.Columns = []datagrid.Column{{Key: key, Label: key, Sortable: true}}
			_, err := New(d)
			require.NoError(t, err, "request control names remain valid readable and sortable columns")
			d.Columns[0].Filterable = true
			_, err = New(d)
			if key == "name" || key == "toString" || key == "valueOf" || key == "ids" {
				require.NoError(t, err, "ordinary fields remain filterable")
			} else {
				require.ErrorContains(t, err, "filterable column key is reserved by datagrid")
			}
		})
	}
	// Synthetic search metadata is already excluded by CRUD field syntax.
	d.Fields = []Field{{Key: "_search", Label: "Search", Type: "text", Read: true}}
	d.Columns = []datagrid.Column{{Key: "_search", Label: "Search", Sortable: true, Filterable: true}}
	_, err := New(d)
	require.ErrorContains(t, err, "invalid or reserved field key")
}

func TestConstructorRejectsVueReactiveHasOwnPropertyField(t *testing.T) {
	d := queryControlDefinition()
	// Vue's reactive proxy intercepts this name even when it is an own field.
	d.Fields = []Field{{Key: "hasOwnProperty", Label: "Property", Type: "text", Read: true, Create: true, Update: true}}
	d.Columns = []datagrid.Column{{Key: "hasOwnProperty", Label: "Property", Sortable: true}}
	_, err := New(d)
	require.ErrorContains(t, err, "invalid or reserved field key")
}
