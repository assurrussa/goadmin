package crud

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

var (
	keyPattern             = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
	fieldPattern           = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,63}$`)
	pathPattern            = regexp.MustCompile(`^(/[a-z][a-z0-9_-]*)+$`)
	customComponentPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]*/[a-z][a-z0-9._-]*$`)
)

//nolint:gocognit,gocyclo // Constructor validation enumerates independent public contract invariants.
func validate[R, C, U any](d Definition[R, C, U], mode resourceMode) error {
	fail := func(s string) error { return fmt.Errorf("crud: %s", s) }
	if !keyPattern.MatchString(d.Key) || !pathPattern.MatchString(d.BasePath) || strings.TrimSpace(d.Title) == "" {
		return fail("invalid key, base path or title")
	}
	if d.List == nil || d.Get == nil || d.Authorize == nil ||
		(mode == editableResource && (d.LoadSelected == nil || d.Create == nil || d.Update == nil || d.Delete == nil)) {
		return fail("all scoped callbacks and record authorization are required")
	}
	p := d.Permissions
	if p.Read.IsZero() || (mode == editableResource && (p.Create.IsZero() || p.Update.IsZero() || p.Delete.IsZero())) {
		return fail("explicit permissions are required")
	}
	if d.PageSize < 0 || d.PageSize > 100 {
		return fail("page size must be 1..100 or zero")
	}
	if d.DefaultOrder != "" && !validSortOrder(d.DefaultOrder) {
		return fail("invalid default order")
	}
	row, err := jsonFields[R]()
	if err != nil {
		return err
	}
	create, err := jsonFields[C]()
	if err != nil {
		return err
	}
	update, err := jsonFields[U]()
	if err != nil {
		return err
	}
	fields := map[string]Field{}
	for _, f := range d.Fields {
		if mode != editableResource && (f.Create || f.Update || f.Required || f.MaxLength != nil || f.Minimum != nil) {
			return fmt.Errorf("crud: read-only field %q cannot set Create, Update, Required, MaxLength or Minimum", f.Key)
		}
		if err := f.validateConstraintDefinition(); err != nil {
			return fmt.Errorf("crud: field %q: %w", f.Key, err)
		}
		if !validFieldKey(f.Key) || f.Label == "" {
			return fail("invalid or reserved field key or label")
		}
		if _, ok := fields[f.Key]; ok {
			return fail("duplicate field")
		}
		fields[f.Key] = f
		if f.Read && row[f.Key] == nil || f.Create && create[f.Key] == nil || f.Update && update[f.Key] == nil {
			return fail("field missing explicit JSON key on its typed DTO")
		}
		if !f.Read && f.Update && !f.Required && update[f.Key].Kind() != reflect.Pointer {
			return fail("optional write-only update fields require pointer DTO fields")
		}
		switch f.Type {
		case fieldText, fieldTextArea, fieldNumber, fieldBoolean, fieldDate:
		case fieldSelect:
			if len(f.Options) == 0 {
				return fail("select requires fixed options")
			}
		case fieldCustom:
			if !customComponentPattern.MatchString(f.Component) {
				return fail("custom component must match extension/key")
			}
			matched := false
			for _, r := range d.ClientRequirements {
				if strings.HasPrefix(f.Component, r.Key+"/") {
					matched = true
				}
			}
			if !matched {
				return fail("custom component requires matching client extension")
			}
		default:
			return fail("unsupported field type")
		}
		for _, mode := range []struct {
			enabled bool
			fields  map[string]reflect.Type
		}{{f.Read, row}, {f.Create, create}, {f.Update, update}} {
			if mode.enabled && !fieldTypeAllowed(f.Type, mode.fields[f.Key]) {
				return fail("field kind does not match its typed DTO")
			}
		}
		values := map[string]bool{}
		for _, o := range f.Options {
			if values[o.Value] {
				return fail("duplicate select option")
			}
			values[o.Value] = true
		}
	}
	if len(fields) == 0 || len(d.Columns) == 0 {
		return fail("fields and columns required")
	}
	columns := map[string]bool{}
	sortable := false
	for _, c := range d.Columns {
		f, ok := fields[c.Key]
		if !ok || !f.Read || columns[c.Key] {
			return fail("columns must name unique readable fields")
		}
		// Keep filter names separate from shared datagrid request/export controls
		// and synthetic search metadata. These keys remain valid display fields.
		if c.Filterable && slices.Contains([]string{
			"page", "limit", "search", "sortBy", "sortOrder", "_search", "scope", "columns",
		}, c.Key) {
			return fail("filterable column key is reserved by datagrid")
		}
		columns[c.Key] = true
		if c.Sortable && (d.DefaultSort == "" || d.DefaultSort == c.Key) {
			sortable = true
		}
	}
	if !sortable {
		return fail("default sort must name a sortable column")
	}
	if mode == commandResource && len(d.Actions) == 0 {
		return fail("at least one action is required")
	}
	actions := map[string]bool{}
	for _, a := range d.Actions {
		if slices.Contains([]string{
			"view",
			editAction,
			string(Delete),
			string(Create),
			string(Read),
			string(Update),
		},
			a.Key) || !keyPattern.MatchString(a.Key) || a.Label == "" || a.Permission.IsZero() || actions[a.Key] {
			return fail("invalid action")
		}
		switch a.Placement {
		case "", Row, Detail, Both:
		default:
			return fmt.Errorf("crud: action %q has invalid placement", a.Key)
		}
		if err := a.validateInput(); err != nil {
			return fmt.Errorf("crud: action %q: %w", a.Key, err)
		}
		actions[a.Key] = true
	}
	return nil
}

func validFieldKey(key string) bool {
	return fieldPattern.MatchString(key) && !slices.Contains([]string{
		"id", revisionKey, "__proto__", "constructor", "prototype", "hasOwnProperty",
	}, key)
}

func validSortOrder(order string) bool { return order == "asc" || order == "desc" }

func jsonFields[T any]() (map[string]reflect.Type, error) { return structFields(reflect.TypeFor[T]()) }

func structFields(t reflect.Type) (map[string]reflect.Type, error) {
	if t.Kind() != reflect.Struct {
		return nil, errors.New("crud: row and inputs must be structs")
	}
	result := map[string]reflect.Type{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		if f.Anonymous {
			return nil, errors.New("crud: embedded DTO fields unsupported")
		}
		key, options, _ := strings.Cut(f.Tag.Get("json"), ",")
		if key == "-" {
			continue
		}
		if key == "" || result[key] != nil || slices.Contains(strings.Split(options, ","), "string") {
			return nil, errors.New("crud: explicit unique unquoted JSON keys required")
		}
		result[key] = f.Type
	}
	return result, nil
}

func cloneDefinition[R, C, U any](d Definition[R, C, U]) Definition[R, C, U] {
	d.Fields = slices.Clone(d.Fields)
	for i := range d.Fields {
		d.Fields[i].Options = slices.Clone(d.Fields[i].Options)
		if bound := d.Fields[i].MaxLength; bound != nil {
			d.Fields[i].MaxLength = new(*bound)
		}
		if bound := d.Fields[i].Minimum; bound != nil {
			d.Fields[i].Minimum = new(*bound)
		}
	}
	d.Columns = cloneColumns(d.Columns)
	d.Actions = slices.Clone(d.Actions)
	for i := range d.Actions {
		d.Actions[i].Params = slices.Clone(d.Actions[i].Params)
		for j := range d.Actions[i].Params {
			if bound := d.Actions[i].Params[j].MaxLength; bound != nil {
				d.Actions[i].Params[j].MaxLength = new(*bound)
			}
		}
	}
	d.ClientRequirements = slices.Clone(d.ClientRequirements)
	if d.PageSize == 0 {
		d.PageSize = 25
	}
	if d.DefaultOrder == "" {
		d.DefaultOrder = "asc"
	}
	if d.DefaultSort == "" {
		for _, c := range d.Columns {
			if c.Sortable {
				d.DefaultSort = c.Key
				break
			}
		}
	}
	return d
}

func cloneColumns(columns []datagrid.Column) []datagrid.Column {
	cloned := slices.Clone(columns)
	for i, col := range cloned {
		options := slices.Clone(col.FilterOptions)
		for j, option := range options {
			options[j] = maps.Clone(option)
		}
		cloned[i].FilterOptions = options
		badges := maps.Clone(col.Badges)
		for k, badge := range badges {
			badges[k] = maps.Clone(badge)
		}
		cloned[i].Badges = badges
	}
	return cloned
}
