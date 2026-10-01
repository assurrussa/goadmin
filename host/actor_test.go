package host_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	adminhost "github.com/assurrussa/goadmin/host"
	adminsession "github.com/assurrussa/goadmin/infrastructure/core/session"
	authcore "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/models"
)

func TestCurrentActorReturnsNarrowCanonicalIdentity(t *testing.T) {
	t.Parallel()

	const subjectID = "123e4567-e89b-12d3-a456-426614174201"
	app := fiber.New()
	wrapper := adminhost.WrapApp(adminappt.NewAppTest(t).App)
	app.Use(func(c fiber.Ctx) error {
		c.Locals(adminsession.AuthAdminKey.String(), &models.SessionAdmin{
			ID:          42,
			SubjectID:   authcore.MustParseSubjectIDString(subjectID),
			Email:       "must-not-leak@example.test",
			Roles:       []string{"super-admin"},
			Permissions: map[string][]string{"cms": {"publish"}},
		})

		return c.Next()
	})
	app.Get("/", func(c fiber.Ctx) error {
		actor, ok := wrapper.CurrentActor(c)
		require.True(t, ok)
		require.Equal(t, adminhost.Actor{AdminID: 42, SubjectID: subjectID}, actor)

		return c.SendStatus(fiber.StatusNoContent)
	})

	response, err := app.Test(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
	require.Equal(t, fiber.StatusNoContent, response.StatusCode)
}

func TestCurrentActorRejectsMissingOrIncompleteIdentity(t *testing.T) {
	t.Parallel()

	wrapper := adminhost.WrapApp(adminappt.NewAppTest(t).App)
	actor, ok := wrapper.CurrentActor(context.Background())
	require.False(t, ok)
	require.True(t, actor.IsZero())

	var nilWrapper *adminhost.App
	actor, ok = nilWrapper.CurrentActor(context.Background())
	require.False(t, ok)
	require.True(t, actor.IsZero())
}
