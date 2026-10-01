package inertia

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	servererrors "github.com/assurrussa/goadmin/infrastructure/fiber/errors"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
)

func TestCustomErrorGettingHandler_ValidationErrorWithFields(t *testing.T) {
	cause := errors.New("cause")
	err := servererrors.NewValidationError(
		"Validation failed",
		[]servererrors.FieldError{
			servererrors.NewFieldError("email", "Invalid email", "invalid_format"),
			servererrors.NewFieldError("name", "Invalid name", "invalid_name"),
		},
		cause,
	)

	appErr := CustomErrorGettingHandler(err)
	require.NotNil(t, appErr)

	assert.Equal(t, http.StatusBadRequest, appErr.Code)
	assert.Equal(t, "Validation failed", appErr.Message)
	require.ErrorIs(t, appErr.Unwrap(), cause)

	vals := appErr.ValidationErrors()
	require.Len(t, vals, 2)
	assert.Equal(t, []string{"email"}, vals["email"])
	assert.Equal(t, []string{"name"}, vals["name"])

	flash := appErr.FlashErrors()
	require.Len(t, flash, 1)
	assert.Equal(t, goinertia.FlashLevelWarning, flash[0].Level)
	assert.Equal(t, "Validation failed", flash[0].Message)
}

func TestCustomErrorGettingHandler_ValidationErrorWithoutFields(t *testing.T) {
	cause := errors.New("cause")
	err := servererrors.NewValidationError(
		"Validation failed",
		nil,
		cause,
	)

	appErr := CustomErrorGettingHandler(err)
	require.NotNil(t, appErr)

	assert.Equal(t, http.StatusBadRequest, appErr.Code)
	assert.Equal(t, "Validation failed", appErr.Message)
	require.ErrorIs(t, appErr.Unwrap(), cause)
	assert.Empty(t, appErr.ValidationErrors())
	assert.Empty(t, appErr.FlashErrors())
}

func TestCustomErrorGettingHandler_ServerError(t *testing.T) {
	cause := errors.New("cause")
	err := servererrors.NewServerError(419, "test error", cause)

	appErr := CustomErrorGettingHandler(err)
	require.NotNil(t, appErr)

	assert.Equal(t, 419, appErr.Code)
	assert.Equal(t, "test error", appErr.Message)
	require.ErrorIs(t, appErr.Unwrap(), cause)
}

func TestCustomErrorGettingHandler_UnknownError(t *testing.T) {
	appErr := CustomErrorGettingHandler(errors.New("unknown"))
	assert.Nil(t, appErr)
}
