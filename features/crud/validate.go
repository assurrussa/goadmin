package crud

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
)

var (
	keyPattern             = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
	fieldPattern           = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,63}$`)
	pathPattern            = regexp.MustCompile(`^(/[a-z][a-z0-9_-]*)+$`)
	customComponentPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]*/[a-z][a-z0-9._-]*$`)
)

//nolint:gocognit,gocyclo // Constructor validation enumerates independent public contract invariants.
func validate[R, C, U any](d Definition[R, C, U]) error {
	fail := func(s string) error { return fmt.Errorf("crud: %s", s) }
	if !keyPattern.MatchString(d.Key) || !pathPattern.MatchString(d.BasePath) || strings.TrimSpace(d.Title) == "" {
		return fail("invalid key, base path or title")
	}
	if d.List == nil ||
		d.Get == nil ||
		d.LoadSelected == nil ||
		d.Create == nil ||
		d.Update == nil ||
		d.Delete == nil ||
		d.Authorize == nil {
		return fail("all scoped callbacks and record authorization are required")
	}
	for _, p := range []bool{
		d.Permissions.Read.IsZero(),
		d.Permissions.Create.IsZero(),
		d.Permissions.Update.IsZero(),
		d.Permissions.Delete.IsZero(),
	} {
		if p {
			return fail("explicit permissions are required")
		}
	}
	if d.PageSize < 0 || d.PageSize > 100 {
		return fail("page size must be 1..100 or zero")
	}
	if d.DefaultOrder != "" && d.DefaultOrder != "asc" && d.DefaultOrder != "desc" {
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
		if !fieldPattern.MatchString(f.Key) ||
			f.Key == "id" ||
			f.Key == revisionKey ||
			f.Key == "__proto__" ||
			f.Key == "constructor" ||
			f.Key == "prototype" ||
			f.Key == "hasOwnProperty" ||
			f.Label == "" {
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
		case "text", "textarea", "number", "boolean", fieldDate:
		case fieldSelect:
			if len(f.Options) == 0 {
				return fail("select requires fixed options")
			}
		case "custom":
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
		// Keep filter names separate from shared datagrid request controls and
		// its synthetic search metadata. These keys remain valid display fields.
		if c.Filterable && slices.Contains([]string{"page", "limit", "search", "sortBy", "sortOrder", "_search"}, c.Key) {
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
	actions := map[string]bool{}
	for _, a := range d.Actions {
		if slices.Contains([]string{
			"view",
			"edit",
			string(Delete),
			string(Create),
			string(Read),
			string(Update),
		},
			a.Key) || !keyPattern.MatchString(a.Key) || a.Label == "" || a.Permission.IsZero() || a.Run == nil || actions[a.Key] {
			return fail("invalid action")
		}
		actions[a.Key] = true
	}
	return nil
}

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
	}
	d.Columns = slices.Clone(d.Columns)
	d.Actions = slices.Clone(d.Actions)
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
