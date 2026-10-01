package formvalidator

import (
	"github.com/gobuffalo/validate"

	core "github.com/assurrussa/goadmin/infrastructure/core/formvalidator"
)

func Validate(validators ...validate.Validator) *validate.Errors {
	return core.Validate(validators...)
}

func FormatErrors(errors *validate.Errors) string {
	return core.FormatErrors(errors)
}
