package servererrors

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gofiber/fiber/v3"
)

// FieldError представляет ошибку для конкретного поля формы.
type FieldError struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
	Key     string `json:"key,omitempty"`
}

// ValidationError представляет ошибку валидации с field errors.
type ValidationError struct {
	Code    int
	Message string
	Fields  []FieldError
	cause   error
}

// ServerError is used to return custom error codes to pgsqlclient.
type ServerError struct {
	Code    int
	Message string
	cause   error
}

func NewServerError(code int, msg string, err error) *ServerError {
	return &ServerError{
		Code:    code,
		Message: msg,
		cause:   err,
	}
}

// NewValidationError создает ошибку валидации с field errors.
func NewValidationError(msg string, fields []FieldError, err error) *ValidationError {
	return &ValidationError{
		Code:    http.StatusBadRequest,
		Message: msg,
		Fields:  fields,
		cause:   err,
	}
}

// NewFieldError создает ошибку для конкретного поля.
func NewFieldError(field, message, key string) FieldError {
	return FieldError{
		Field:   field,
		Message: message,
		Key:     key,
	}
}

// NewFieldErrorSimple создает простую ошибку поля без key.
func NewFieldErrorSimple(field, message string) FieldError {
	return FieldError{
		Field:   field,
		Message: message,
	}
}

func (s *ServerError) Error() string {
	return fmt.Sprintf("%s: %v", s.Message, s.cause)
}

func (s *ServerError) Unwrap() error {
	return s.cause
}

func (v *ValidationError) Error() string {
	if v.cause != nil {
		return fmt.Sprintf("%s: %v", v.Message, v.cause)
	}
	return v.Message
}

func (v *ValidationError) Unwrap() error {
	return v.cause
}

func GetServerErrorCode(err error) int {
	code, _, _, _ := ProcessServerError(err) //nolint:dogsled // it's need
	return code
}

const (
	defaultCode    = http.StatusInternalServerError
	defaultMessage = "something went wrong"
)

// ProcessServerError tries to retrieve from given error it's code, message, details and field errors.
// For example, that fields can be used to build error response for pgsqlclient.
func ProcessServerError(err error) (code int, msg string, details string, fieldErrors []FieldError) {
	defer func() {
		if code == 0 {
			code = defaultCode
		}

		if msg == "" {
			msg = defaultMessage
		}
	}()

	// Проверяем ValidationError
	if errValidation := new(ValidationError); errors.As(err, &errValidation) {
		return errValidation.Code, errValidation.Message, errValidation.Error(), errValidation.Fields
	}

	// Проверяем ServerError
	if errSrv := new(ServerError); errors.As(err, &errSrv) {
		return errSrv.Code, errSrv.Message, errSrv.Error(), nil
	}

	// Проверяем fiber.Error
	if errHTTP := new(fiber.Error); errors.As(err, &errHTTP) {
		return errHTTP.Code, errHTTP.Message, errHTTP.Error(), nil
	}

	return code, msg, err.Error(), nil
}
