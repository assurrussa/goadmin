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
	fieldDate   = "date"
	fieldSelect = "select"
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
	case "text", "textarea", fieldDate, fieldSelect:
		return k == reflect.String
	case "boolean":
		return k == reflect.Bool
	case "number":
		return numericKind(k)
	case "custom":
		return k == reflect.String || k == reflect.Bool || numericKind(k)
	}
	return false
}

func validateFieldValue(f Field, t reflect.Type, raw json.RawMessage) error {
	invalid := func() error { return errors.New("invalid field value") }
	if t == nil {
		return invalid()
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if !f.Required && t.Kind() == reflect.Pointer {
			return nil
		}
		return invalid()
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return invalid()
	}
	expected := scalarType(t).Kind()
	switch v := value.(type) {
	case string:
		if expected != reflect.String {
			return invalid()
		}
		return validateStringValue(f, v)
	case bool:
		if expected != reflect.Bool {
			return invalid()
		}
	case json.Number:
		if !numericKind(expected) {
			return invalid()
		}
		return validateNumericValue(v)
	default:
		return invalid()
	}
	return nil
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

func validateNumericValue(value json.Number) error {
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
		return nil
	}
	exact, ok := new(big.Rat).SetString(string(value))
	if !ok || new(big.Rat).Abs(exact).Cmp(new(big.Rat).SetInt64(9007199254740991)) > 0 {
		return errors.New("number exceeds the safe range")
	}
	return nil
}
