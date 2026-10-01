package csrf

import (
	"github.com/go-playground/validator/v10"

	"github.com/assurrussa/goadmin/internal/identity"
)

var validate = validator.New()

type Request struct {
	SessionID string `validate:"required"`
	UserID    identity.UserID
	Domain    string
}

func (r Request) Validate() error {
	return validate.Struct(r)
}

// Token represents CSRF token data.
type Token struct {
	Domain    string
	Token     string
	SessionID string
	UserID    identity.UserID
}
