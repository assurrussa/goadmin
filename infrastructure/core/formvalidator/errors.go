package formvalidator

import (
	"strings"

	"github.com/gobuffalo/validate"
)

func Validate(validators ...validate.Validator) *validate.Errors {
	return validate.Validate(validators...)
}

// FormatErrors красиво форматирует ошибки валидации в одну строку для фронта.
func FormatErrors(errors *validate.Errors) string {
	if errors == nil || len(errors.Errors) == 0 {
		return ""
	}

	uniq := make(map[string]struct{})
	var out []string
	for _, group := range errors.Errors {
		for _, msg := range group {
			msg = strings.TrimSpace(msg)
			if msg == "" {
				continue
			}
			if _, ok := uniq[msg]; !ok {
				uniq[msg] = struct{}{}
				out = append(out, msg)
			}
		}
	}

	if len(out) == 0 {
		return ""
	}

	return strings.Join(out, ", ") + "."
}
