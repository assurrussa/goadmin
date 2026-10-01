package inertia

import (
	"errors"

	servererrors "github.com/assurrussa/goadmin/infrastructure/fiber/errors"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
)

func CustomErrorGettingHandler(err error) *goinertia.Error {
	if errHTTP := new(servererrors.ValidationError); errors.As(err, &errHTTP) {
		err := goinertia.NewError(errHTTP.Code, errHTTP.Message, errHTTP.Unwrap())
		if len(errHTTP.Fields) == 0 {
			return err
		}

		vals := make(goinertia.ValidationErrors, len(errHTTP.Fields))
		for _, field := range errHTTP.Fields {
			vals[field.Field] = []string{field.Field}
		}

		return err.CloneValidationError(goinertia.NewValidationError(errHTTP.Code, errHTTP.Message, vals)).
			WithFlashErrors(goinertia.NewFlashError(goinertia.FlashLevelWarning, errHTTP.Message))
	}

	if errHTTP := new(servererrors.ServerError); errors.As(err, &errHTTP) {
		errCause := errHTTP.Unwrap()
		return goinertia.NewError(errHTTP.Code, errHTTP.Message, errCause)
	}

	return nil
}
