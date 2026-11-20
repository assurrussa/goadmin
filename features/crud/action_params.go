package crud

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/assurrussa/goadmin/host"
)

// ActionParam declares one text or textarea command argument. Its Key is the
// explicit JSON tag on a string field in the action's input struct.
type ActionParam struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Type      string `json:"type"`
	Required  bool   `json:"required,omitempty"`
	TrimSpace bool   `json:"trimSpace,omitempty"`
	MaxLength *int   `json:"maxLength,omitempty"`
}

// Normalize returns the value that can be persisted by the host. White_Space
// trimming precedes the required check and the Unicode code point limit.
func (p ActionParam) Normalize(value string) (string, error) {
	if !utf8.ValidString(value) || strings.IndexByte(value, 0) >= 0 {
		return "", errors.New(invalidFieldMessage) //nolint:staticcheck // User-facing validation message.
	}
	if p.TrimSpace {
		value = strings.TrimFunc(value, unicode.IsSpace)
	}
	if p.Required && value == "" {
		return "", errors.New("This field is required") //nolint:staticcheck // User-facing validation message.
	}
	if err := (Field{MaxLength: p.MaxLength}).ValidateConstraints(value); err != nil {
		return "", err
	}
	return value, nil
}

type actionInput struct {
	dto reflect.Type
	run func(context.Context, host.Actor, string, string, map[string]string) error
}

// ActionWithParams attaches typed command input to an action. Leave its ordinary
// Run callback unset. New checks that params match the exported JSON fields of P.
func ActionWithParams[R, P any](action Action[R], params []ActionParam,
	run func(context.Context, host.Actor, string, string, P) error,
) Action[R] {
	action.Params = params
	action.input = &actionInput{dto: reflect.TypeFor[P]()}
	if run != nil {
		action.input.run = func(ctx context.Context, actor host.Actor, id, revision string, values map[string]string) error {
			var input P
			dto := reflect.ValueOf(&input).Elem()
			for i := range dto.NumField() {
				field := dto.Type().Field(i)
				key, _, _ := strings.Cut(field.Tag.Get("json"), ",")
				if value, ok := values[key]; ok && field.IsExported() {
					dto.Field(i).SetString(value)
				}
			}
			return run(ctx, actor, id, revision, input)
		}
	}
	return action
}

func (a Action[R]) validateInput() error {
	if a.input == nil {
		if a.Run == nil || len(a.Params) != 0 {
			return errors.New("ordinary action requires Run and cannot declare params")
		}
		return nil
	}
	if a.Run != nil || a.input.run == nil || len(a.Params) == 0 {
		return errors.New("parameterized action requires params and only a typed callback")
	}
	fields, err := structFields(a.input.dto)
	if err != nil {
		return err
	}
	for _, p := range a.Params {
		t := fields[p.Key]
		if !validFieldKey(p.Key) || p.Label == "" || t == nil || t.Kind() != reflect.String {
			return fmt.Errorf("invalid or duplicate parameter %q: expected a labelled string field with matching JSON tag", p.Key)
		}
		if (p.Type != fieldText && p.Type != fieldTextArea) || (p.MaxLength != nil && *p.MaxLength <= 0) {
			return fmt.Errorf("invalid text type or length for parameter %q", p.Key)
		}
		delete(fields, p.Key)
	}
	if len(fields) != 0 {
		return errors.New("every JSON field of the action input must be declared")
	}
	return nil
}
