package crud

import (
	"encoding/json"
	"reflect"
	"strings"
)

func projectFields[R any](fields []Field, r Record[R], op Operation) (map[string]any, error) {
	encoded, err := json.Marshal(r.Data)
	if err != nil {
		return nil, err
	}
	var all map[string]json.RawMessage
	if err = json.Unmarshal(encoded, &all); err != nil {
		return nil, err
	}
	types, err := jsonFields[R]()
	if err != nil {
		return nil, err
	}
	_, customRowJSON := any(r.Data).(json.Marshaler)
	completeEdit := op == Update && !customRowJSON
	out := make(map[string]any, len(fields))
	var zeroValues map[string]reflect.Value
	for _, f := range fields {
		if !f.Read {
			continue
		}
		fieldType := types[f.Key]
		value, present := all[f.Key]
		f.Required = false
		if present {
			scalar, err := fieldValue(f, fieldType, value)
			if err != nil {
				return nil, err
			}
			out[f.Key] = scalar
			continue
		}
		if !completeEdit || !f.Update || fieldType == nil {
			continue
		}
		// IsZero methods can make the standard encoder omit nonzero fields.
		// Restore a zero only when it represents the actual DTO value.
		if zeroValues == nil {
			zeroValues = rowZeroValues(r.Data)
		}
		if zero := zeroValues[f.Key]; zero.IsValid() {
			value, err = json.Marshal(zero.Interface())
			if scalar, validationErr := fieldValue(f, fieldType, value); err == nil && validationErr == nil {
				out[f.Key] = scalar
			}
		}
	}
	return out, nil
}

func rowZeroValues[R any](data R) map[string]reflect.Value {
	row := reflect.ValueOf(data)
	fields := make(map[string]reflect.Value)
	for i := range row.NumField() {
		value := row.Field(i)
		if field := row.Type().Field(i); field.IsExported() && value.IsZero() {
			key, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			fields[key] = value
		}
	}
	return fields
}
