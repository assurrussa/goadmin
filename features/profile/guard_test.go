package profile //nolint:testpackage // Verify preview fence precedes private service access.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	authcore "github.com/assurrussa/goadmin/internal/auth"
)

func TestPreviewDeniesProfileMutationBeforeBindingOrService(t *testing.T) {
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.SetContext(authcore.WithSuperRolePreview(c.Context()))
		return c.Next()
	})
	h := &handler{}
	for _, path := range []string{sourcePath, sourcePath+refreshSuffix, sourcePath+cancelSuffix} {
		app.Post(path, func(c fiber.Ctx) error {
			_, _, err := h.input(c)
			require.ErrorIs(t, err, ErrDenied)
			return c.SendStatus(http.StatusForbidden)
		})
	}
	for _, path := range []string{sourcePath, sourcePath+refreshSuffix, sourcePath+cancelSuffix} {
		response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, nil))
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		require.Equal(t, http.StatusForbidden, response.StatusCode)
	}
}
