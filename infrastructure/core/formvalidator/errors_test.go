package formvalidator_test

import (
	"strings"
	"testing"

	"github.com/gobuffalo/validate"

	"github.com/assurrussa/goadmin/infrastructure/core/formvalidator"
)

func splitAndTrim(s string) map[string]struct{} {
	res := map[string]struct{}{}
	s = strings.TrimSpace(s)

	if s == "" {
		return res
	}
	s = strings.TrimSuffix(s, ".")
	for _, part := range strings.Split(s, ",") {
		msg := strings.TrimSpace(part)
		if msg != "" {
			res[msg] = struct{}{}
		}
	}
	return res
}

func checkFormatErrors(t *testing.T, got, expect string) {
	t.Helper()

	expectSet := splitAndTrim(expect)
	gotSet := splitAndTrim(got)

	if len(expectSet) != len(gotSet) {
		t.Errorf("FormatErrors() set size = %d, want %d", len(gotSet), len(expectSet))
	}
	for msg := range expectSet {
		if _, ok := gotSet[msg]; !ok {
			t.Errorf("FormatErrors() missing error: %q", msg)
		}
	}
	for msg := range gotSet {
		if _, ok := expectSet[msg]; !ok {
			t.Errorf("FormatErrors() unexpected error: %q", msg)
		}
	}

	if len(gotSet) > 0 && !strings.HasSuffix(got, ".") {
		t.Errorf("FormatErrors() output should end with a period if not empty: %q", got)
	}
	if len(gotSet) == 0 && got != "" {
		t.Errorf("FormatErrors() should be empty string if no errors, got: %q", got)
	}
}

func TestFormatErrors(t *testing.T) {
	tests := []struct {
		name   string
		input  *validate.Errors
		expect string
	}{
		{
			name:   "empty map",
			input:  nil,
			expect: "",
		},
		{
			name:   "empty map 2",
			input:  &validate.Errors{},
			expect: "",
		},
		{
			name:   "empty map 3",
			input:  &validate.Errors{Errors: map[string][]string{}},
			expect: "",
		},
		{
			name: "all empty strings",
			input: &validate.Errors{Errors: map[string][]string{
				"a": {""},
				"b": {"  "},
			}},
			expect: "",
		},
		{
			name: "single error",
			input: &validate.Errors{Errors: map[string][]string{
				"a": {"Ошибка 1"}, //nolint:goconst // required
			}},
			expect: "Ошибка 1.",
		},
		{
			name: "multiple errors, no dups",
			input: &validate.Errors{Errors: map[string][]string{
				"a": {"Ошибка 1"},
				"b": {"Ошибка 2"}, //nolint:goconst // required
			}},
			expect: "Ошибка 1, Ошибка 2.", //nolint:goconst // required
		},
		{
			name: "multiple errors, with dups",
			input: &validate.Errors{Errors: map[string][]string{
				"a": {"Ошибка 1", "Ошибка 2"},
				"b": {"Ошибка 1"},
			}},
			expect: "Ошибка 1, Ошибка 2.",
		},
		{
			name: "trims and skips empty",
			input: &validate.Errors{Errors: map[string][]string{
				"a": {"Ошибка 1", ""},
				"b": {"Ошибка 2", " "},
			}},
			expect: "Ошибка 1, Ошибка 2.",
		},
		{
			name: "complex",
			input: &validate.Errors{Errors: map[string][]string{
				"a": {"Ошибка 1", "Ошибка 2"},
				"b": {"Ошибка 3", "Ошибка 1", "Ошибка 2"},
				"c": {"Ошибка 4"},
			}},
			expect: "Ошибка 1, Ошибка 2, Ошибка 3, Ошибка 4.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formvalidator.FormatErrors(tt.input)
			checkFormatErrors(t, got, tt.expect)
		})
	}
}

func TestValidate(t *testing.T) {
	t.Run("no validators", func(t *testing.T) {
		err := formvalidator.Validate()
		if err == nil || len(err.Errors) != 0 {
			t.Errorf("expected empty errors, got: %#v", err)
		}
	})

	t.Run("one validator with error", func(t *testing.T) {
		v := validate.ValidatorFunc(func(errors *validate.Errors) {
			errors.Add("field", "err")
		})
		err := formvalidator.Validate(v)
		if err == nil || len(err.Errors) == 0 {
			t.Errorf("expected error, got: %#v", err)
		}
		if err.Errors["field"][0] != "err" {
			t.Errorf("expected 'err', got: %#v", err.Errors["field"][0])
		}
	})

	t.Run("multiple validators, mixed", func(t *testing.T) {
		v1 := validate.ValidatorFunc(func(errors *validate.Errors) {
			errors.Add("f1", "e1")
		})
		v2 := validate.ValidatorFunc(func(_ *validate.Errors) {
			// no error
		})
		v3 := validate.ValidatorFunc(func(errors *validate.Errors) {
			errors.Add("f2", "e2")
		})
		err := formvalidator.Validate(v1, v2, v3)
		if err == nil || len(err.Errors) != 2 {
			t.Errorf("expected 2 errors, got: %#v", err)
		}
		if err.Errors["f1"][0] != "e1" || err.Errors["f2"][0] != "e2" {
			t.Errorf("unexpected errors: %#v", err.Errors)
		}
	})
}
