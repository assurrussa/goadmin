//nolint:goconst // Literal values describe the column declaration and DataGrid contract.
package crud //nolint:testpackage // Verify normalization and constructor validation.

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

func TestListColumnsDeriveGridMetadata(t *testing.T) {
	fields := []Field{
		{Key: "title", Label: "Title", Type: fieldText, Read: true},
		{Key: "status", Label: "Status", Type: fieldSelect, Read: true, Options: []Option{{Value: "open", Label: "Open"}}},
		{Key: "closedAt", Label: "Closed at", Type: fieldDate, Read: true},
		{Key: "notes", Label: "Notes", Type: fieldTextArea, Read: true},
	}
	badges := map[string]map[string]string{"open": {"label": "Open", "variant": "success"}}
	columns, err := deriveColumns(fields, []ListColumn{
		{Key: "status", Type: "badge", Filterable: true, Badges: badges},
		{Key: "title", Filterable: true, Sortable: true, FilterPlaceholder: "Filter title"},
		{Key: "closedAt", Label: "Closing date", Format: "DD.MM.YYYY"},
		{Key: "notes"},
	})
	require.NoError(t, err)
	require.Equal(t, []datagrid.Column{
		{
			Key: "status", Label: "Status", Type: "badge", Filterable: true, Badges: badges,
			FilterOptions: []map[string]string{{"value": "open", "label": "Open"}},
		},
		{Key: "title", Label: "Title", Type: fieldText, Filterable: true, Sortable: true, FilterPlaceholder: "Filter title"},
		{Key: "closedAt", Label: "Closing date", Type: fieldDate, Format: "DD.MM.YYYY"},
		{Key: "notes", Label: "Notes", Type: fieldText},
	}, columns)
	selectColumn, err := deriveColumns(fields, []ListColumn{{Key: "status", Filterable: true}})
	require.NoError(t, err)
	require.Equal(t, fieldSelect, selectColumn[0].Type)
	require.Equal(t, columns[0].FilterOptions, selectColumn[0].FilterOptions)
}

func TestConstructorsAcceptListColumns(t *testing.T) {
	d := definition()
	d.Columns = nil
	d.ListColumns = []ListColumn{{Key: "name", Sortable: true, Filterable: true}}
	_, err := New(d)
	require.NoError(t, err)

	readOnly := exampleEvents()
	readOnly.Columns = nil
	readOnly.ListColumns = []ListColumn{{Key: "label", Sortable: true}}
	_, err = NewReadOnly(readOnly)
	require.NoError(t, err)

	commands := commandDefinition()
	commands.Columns = nil
	commands.ListColumns = d.ListColumns
	_, err = NewWithActions(commands)
	require.NoError(t, err)
}

func TestListColumnsRejectInvalidDefinitions(t *testing.T) {
	for name, change := range map[string]func(*Definition[testRow, testCreate, testUpdate]){
		"two representations": func(d *Definition[testRow, testCreate, testUpdate]) { d.Columns = definition().Columns },
		"unknown field":       func(d *Definition[testRow, testCreate, testUpdate]) { d.ListColumns[0].Key = "missing" },
		"unreadable field":    func(d *Definition[testRow, testCreate, testUpdate]) { d.Fields[0].Read = false },
		"duplicate key": func(d *Definition[testRow, testCreate, testUpdate]) {
			d.ListColumns = append(d.ListColumns, d.ListColumns[0])
		},
		"unsupported type": func(d *Definition[testRow, testCreate, testUpdate]) { d.ListColumns[0].Type = "invalid" },
		"select without options": func(d *Definition[testRow, testCreate, testUpdate]) {
			d.ListColumns[0].Type = fieldSelect
		},
		"default sort outside projection": func(d *Definition[testRow, testCreate, testUpdate]) { d.DefaultSort = "readOnly" },
	} {
		t.Run(name, func(t *testing.T) {
			d := definition()
			d.Columns = nil
			d.ListColumns = []ListColumn{{Key: "name", Sortable: true, Filterable: true}}
			change(&d)
			_, err := New(d)
			require.Error(t, err)
		})
	}
}
