package crud

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"unicode/utf8"
)

const maxSafeNumber = 1<<53 - 1

var errBelowMinimum = errors.New("Value is below the minimum")

func (f Field) validateConstraintDefinition() error {
	switch {
	case (f.MaxLength != nil || f.Minimum != nil) && !f.Create && !f.Update:
		return errors.New("write constraints require a writable field")
	case f.MaxLength != nil && (f.Type != fieldText && f.Type != fieldTextArea || *f.MaxLength <= 0):
		return errors.New("expected positive MaxLength on text or textarea")
	case f.Minimum != nil && (f.Type != fieldNumber || math.IsNaN(*f.Minimum) || math.Abs(*f.Minimum) > maxSafeNumber):
		return errors.New("expected finite Minimum within the safe range on a number field")
	}
	return nil
}

// ValidateConstraints checks the declared MaxLength and Minimum on one write
// value. It accepts scalar Go values, including named types and pointers, and
// json.Number. Optional nil values have no length or magnitude to validate.
func (f Field) ValidateConstraints(value any) error {
	if f.MaxLength == nil && f.Minimum == nil {
		return nil
	}
	v := reflect.ValueOf(value)
	for v.IsValid() && v.Kind() == reflect.Pointer {
		if v.IsNil() {
			t := scalarType(v.Type())
			numeric := t == reflect.TypeFor[json.Number]() || numericKind(t.Kind())
			if (f.Minimum != nil && !numeric) || (f.MaxLength != nil && (numeric || t.Kind() != reflect.String)) {
				return errors.New("invalid field value")
			}
		}
		v = v.Elem()
	}
	if !v.IsValid() {
		if f.Required {
			return errors.New("this field is required")
		}
		return nil
	}
	if number, ok := v.Interface().(json.Number); ok {
		if f.Minimum == nil || !json.Valid([]byte(number)) {
			return errors.New("invalid field value")
		}
		return validateNumericValue(number, f.Minimum)
	}
	if f.MaxLength != nil {
		if v.Kind() != reflect.String {
			return errors.New("invalid field value")
		}
		if utf8.RuneCountInString(v.String()) > *f.MaxLength {
			return errors.New("Text is too long") //nolint:staticcheck // User-facing validation message.
		}
		return nil
	}
	return validateGoNumber(v, *f.Minimum)
}

func validateGoNumber(v reflect.Value, minimum float64) error {
	// All integers in the allowed range are exactly representable in float64.
	// Promotion of a float32 also preserves the exact value delivered to the host.
	var number float64
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		number = float64(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		number = float64(v.Uint())
	case reflect.Float32, reflect.Float64:
		number = v.Float()
	default:
		return errors.New("invalid field value")
	}
	switch {
	case math.IsNaN(number) || math.IsInf(number, 0):
		return errors.New("invalid field value")
	case math.Abs(number) > maxSafeNumber:
		return errors.New("number exceeds the safe range")
	case math.IsNaN(minimum) || math.Abs(minimum) > maxSafeNumber:
		return errors.New("invalid Minimum")
	case number < minimum:
		return errBelowMinimum
	}
	return nil
}
