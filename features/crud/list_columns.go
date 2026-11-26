package crud

import (
	"fmt"

	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

// ListColumn references a readable Field by key. Label and Type default to the
// field declaration, with textarea and custom displayed as text by default.
// Select filter options come from Field.Options.
type ListColumn struct {
	Key               string
	Sortable          bool
	Filterable        bool
	Label             string
	Type              string
	Format            string
	FilterPlaceholder string
	Badges            map[string]map[string]string
}

func deriveColumns(fields []Field, list []ListColumn) ([]datagrid.Column, error) {
	byKey := make(map[string]Field, len(fields))
	for _, field := range fields {
		byKey[field.Key] = field
	}
	columns := make([]datagrid.Column, 0, len(list))
	for _, item := range list {
		field, ok := byKey[item.Key]
		if !ok || !field.Read {
			return nil, fmt.Errorf("crud: list column %q must name a readable field", item.Key)
		}
		column := datagrid.Column{
			Key: item.Key, Label: item.Label, Type: item.Type, Format: item.Format,
			Sortable: item.Sortable, Filterable: item.Filterable,
			FilterPlaceholder: item.FilterPlaceholder, Badges: item.Badges,
		}
		if column.Label == "" {
			column.Label = field.Label
		}
		if column.Type == "" {
			column.Type = field.Type
			if column.Type == fieldTextArea || column.Type == fieldCustom {
				column.Type = fieldText
			}
		}
		switch column.Type {
		case fieldText, fieldNumber, fieldBoolean, fieldDate, fieldSelect, "badge", "richtext":
		default:
			return nil, fmt.Errorf("crud: list column %q has unsupported grid type", item.Key)
		}
		if column.Filterable && column.Type == fieldSelect && len(field.Options) == 0 {
			return nil, fmt.Errorf("crud: select filter %q requires field options", item.Key)
		}
		if column.Filterable && (column.Type == fieldSelect || column.Type == "badge") {
			for _, option := range field.Options {
				column.FilterOptions = append(column.FilterOptions, map[string]string{"value": option.Value, "label": option.Label})
			}
		}
		columns = append(columns, column)
	}
	return columns, nil
}
