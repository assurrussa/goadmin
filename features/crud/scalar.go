package crud

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"time"
)

const (
	fieldText     = "text"
	fieldTextArea = "textarea"
	fieldNumber   = "number"
	fieldBoolean  = "boolean"
	fieldDate     = "date"
	fieldSelect   = "select"
)

func scalarType(t reflect.Type) reflect.Type {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

func numericKind(k reflect.Kind) bool {
	return k >= reflect.Int && k <= reflect.Float64 && k != reflect.Uintptr && k != reflect.Complex64 && k != reflect.Complex128
}

func fieldTypeAllowed(kind string, t reflect.Type) bool {
	t = scalarType(t)
	if t == nil {
		return false
	}
	k := t.Kind()
	switch kind {
	case fieldText, fieldTextArea, fieldDate, fieldSelect:
		return k == reflect.String
	case fieldBoolean:
		return k == reflect.Bool
	case fieldNumber:
		return numericKind(k)
	case "custom":
		return k == reflect.String || k == reflect.Bool || numericKind(k)
	}
	return false
}

func validateFieldValue(f Field, t reflect.Type, raw json.RawMessage) error {
	if raw == nil {
		if f.Required {
			return errors.New("This field is required") //nolint:staticcheck // Existing user-facing message for an omitted key.
		}
		if f.Minimum != nil && t.Kind() != reflect.Pointer {
			// The decoder gives an omitted non-pointer number its zero value.
			return f.ValidateConstraints(0)
		}
		return nil
	}
	v, err := decodeScalar(f, t, raw)
	if err != nil {
		return err
	}
	if number, ok := v.(json.Number); ok && f.Minimum == nil {
		return validateNumericValue(number, nil)
	}
	return f.ValidateConstraints(v)
}

// fieldValue preserves the DTO's scalar contract and returns transport-neutral
// values for readers. An omitted key remains distinct from a present null.
func fieldValue(f Field, t reflect.Type, raw json.RawMessage) (any, error) {
	value, err := decodeScalar(f, t, raw)
	if err != nil {
		return nil, err
	}
	number, numeric := value.(json.Number)
	if !numeric {
		return value, nil
	}
	if err := validateNumericValue(number, nil); err != nil {
		return nil, err
	}
	if integer, err := number.Int64(); err == nil {
		return integer, nil
	}
	return number.Float64()
}

func decodeScalar(f Field, t reflect.Type, raw json.RawMessage) (any, error) {
	if t == nil {
		return nil, errors.New("invalid field type")
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if !f.Required && t.Kind() == reflect.Pointer {
			return nil, nil //nolint:nilnil // JSON null is a valid value for an optional pointer field.
		}
		return nil, errors.New("invalid null value")
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, errors.New("invalid field value")
	}
	expected := scalarType(t).Kind()
	switch v := value.(type) {
	case string:
		if expected == reflect.String {
			return v, validateStringValue(f, v)
		}
	case bool:
		if expected == reflect.Bool {
			return v, nil
		}
	case json.Number:
		if numericKind(expected) {
			return v, nil
		}
	}
	return nil, errors.New("invalid field value")
}

func validateStringValue(f Field, value string) error {
	if f.Required && value == "" {
		return errors.New("this field is required")
	}
	if f.Type == fieldDate && value != "" {
		date, err := time.Parse("2006-01-02", value)
		if err != nil || date.Format("2006-01-02") != value || date.Year() < 1 {
			return errors.New("use YYYY-MM-DD")
		}
	}
	if f.Type == fieldSelect {
		if value == "" && !f.Required {
			return nil
		}
		for _, option := range f.Options {
			if option.Value == value {
				return nil
			}
		}
		return errors.New("choose an allowed option")
	}
	return nil
}

func validateNumericValue(value json.Number, minimum *float64) error {
	number, err := strconv.ParseFloat(string(value), 64)
	if err != nil || math.IsInf(number, 0) || math.IsNaN(number) {
		return errors.New("invalid field value")
	}
	// Reject nonzero underflow before exact parsing. Besides losing the value
	// in JavaScript, an unbounded negative exponent must not allocate a huge Rat.
	if number == 0 {
		mantissa, _, _ := strings.Cut(strings.ToLower(string(value)), "e")
		if strings.Trim(mantissa, "-+.0") != "" {
			return errors.New("number cannot be represented safely")
		}
		value = "0"
	}
	exact, ok := new(big.Rat).SetString(string(value))
	if !ok || new(big.Rat).Abs(exact).Cmp(new(big.Rat).SetInt64(maxSafeNumber)) > 0 {
		return errors.New("number exceeds the safe range")
	}
	if minimum != nil {
		// Compare with the same decimal bound published in the form metadata.
		bound, valid := new(big.Rat).SetString(strconv.FormatFloat(*minimum, 'g', -1, 64))
		if !valid {
			return errors.New("invalid Minimum")
		}
		if exact.Cmp(bound) < 0 {
			return errBelowMinimum
		}
	}
	return nil
}
