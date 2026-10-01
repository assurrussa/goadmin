package servererrors_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	servererrors "github.com/assurrussa/goadmin/infrastructure/fiber/errors"
)

func TestProcessWrappedErrors(t *testing.T) {
	cause := errors.New("storage rejected input")
	fields := []servererrors.FieldError{servererrors.NewFieldError("email", "already used", "unique")}
	err := servererrors.NewValidationError("invalid input", fields, cause)
	code, message, details, gotFields := servererrors.ProcessServerError(fmt.Errorf("handler: %w", err))
	require.Equal(t, http.StatusBadRequest, code)
	require.Equal(t, "invalid input", message)
	require.Contains(t, details, cause.Error())
	require.Equal(t, fields, gotFields)
	require.ErrorIs(t, err, cause)

	errServer := servererrors.NewServerError(http.StatusForbidden, "forbidden", cause)
	require.Equal(t, http.StatusForbidden, servererrors.GetServerErrorCode(fmt.Errorf("handler: %w", errServer)))
	require.ErrorIs(t, errServer, cause)
	require.Equal(t, http.StatusNotFound, servererrors.GetServerErrorCode(fiber.ErrNotFound))
	require.Equal(t, http.StatusInternalServerError, servererrors.GetServerErrorCode(cause))
}
