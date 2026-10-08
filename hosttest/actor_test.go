package hosttest_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/hosttest"
)

const testSubjectID = "123e4567-e89b-12d3-a456-426614174201"

func TestWithActorSuppliesCanonicalIdentity(t *testing.T) {
	t.Parallel()

	want := host.Actor{AdminID: 42, SubjectID: testSubjectID}
	input := host.Actor{AdminID: want.AdminID, SubjectID: strings.ToUpper(testSubjectID)}
	withActor, err := hosttest.WithActor(input)
	require.NoError(t, err)

	app := fiber.New()
	wrapper := host.WrapApp(hosttest.NewApp(t).App)
	currentActor := func(c fiber.Ctx) error {
		actor, ok := wrapper.CurrentActor(c)
		if !ok {
			return fiber.ErrUnauthorized
		}
		return c.JSON(actor)
	}
	app.Get("/actor", withActor, currentActor)
	app.Get("/anonymous", currentActor)

	for range 2 {
		response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/actor", nil))
		require.NoError(t, err)
		var got host.Actor
		decodeErr := json.NewDecoder(response.Body).Decode(&got)
		closeErr := response.Body.Close()
		require.NoError(t, decodeErr)
		require.NoError(t, closeErr)
		require.Equal(t, http.StatusOK, response.StatusCode)
		require.Equal(t, want, got)
	}

	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/anonymous", nil))
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
}

func TestWithActorRejectsInvalidIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		actor host.Actor
	}{
		{name: "empty"},
		{name: "zero admin ID", actor: host.Actor{SubjectID: testSubjectID}},
		{name: "negative admin ID", actor: host.Actor{AdminID: -1, SubjectID: testSubjectID}},
		{name: "empty subject ID", actor: host.Actor{AdminID: 42}},
		{name: "malformed subject ID", actor: host.Actor{AdminID: 42, SubjectID: "invalid"}},
		{name: "zero subject ID", actor: host.Actor{AdminID: 42, SubjectID: "00000000-0000-0000-0000-000000000000"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			handler, err := hosttest.WithActor(tt.actor)
			require.Error(t, err)
			require.Nil(t, handler)
		})
	}
}

func TestWithActorCommandContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		legacy        bool
		deny          bool
		body          string
		wantStatus    int
		wantCalled    bool
		wantError     error
		wantBindError bool
	}{
		{name: "typed command", body: `{"name":"updated"}`, wantStatus: http.StatusOK, wantCalled: true},
		{
			name: "legacy session remains incomplete", legacy: true, body: "{",
			wantStatus: http.StatusUnauthorized, wantError: fiber.ErrUnauthorized,
		},
		{
			name: "guard rejects before binding", deny: true, body: "{",
			wantStatus: http.StatusForbidden, wantError: fiber.ErrForbidden,
		},
		// Command propagates BindError; Fiber's default mapper renders it as 500.
		{
			name: "invalid body returns binding error", body: "{",
			wantStatus: http.StatusInternalServerError, wantBindError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			harness := hosttest.NewApp(t)
			key := host.NewPermissionKey(host.PermissionDomainUsers, host.PermissionActionUpdate)
			guardCalled := false
			harness.ExpertGuard(key.Domain, key.Action, func(c fiber.Ctx) error {
				guardCalled = true
				if tt.deny {
					return fiber.ErrForbidden
				}
				return c.Next()
			})
			wrapper := host.WrapApp(harness.App)
			wantActor := host.Actor{AdminID: 42, SubjectID: testSubjectID}
			withActor, err := hosttest.WithActor(wantActor)
			require.NoError(t, err)
			if tt.legacy {
				withActor = hosttest.WithSessionAdmin(wantActor.AdminID)
			}

			type command struct {
				Name string `json:"name"`
			}
			type result struct {
				Actor host.Actor `json:"actor"`
				Name  string     `json:"name"`
			}
			called := false
			var commandErr error
			app := fiber.New(fiber.Config{ErrorHandler: func(c fiber.Ctx, err error) error {
				commandErr = err
				return fiber.DefaultErrorHandler(c, err)
			}})
			app.Use(withActor)
			handlers := host.Command(wrapper, key, func(c fiber.Ctx, actor host.Actor, input command) error {
				called = true
				return c.JSON(result{Actor: actor, Name: input.Name})
			})
			app.Post("/", handlers[0], handlers[1])
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			response, err := app.Test(request)
			require.NoError(t, err)
			var got result
			if tt.wantCalled {
				err = json.NewDecoder(response.Body).Decode(&got)
			}
			closeErr := response.Body.Close()
			require.NoError(t, err)
			require.NoError(t, closeErr)
			require.Equal(t, tt.wantStatus, response.StatusCode)
			require.True(t, guardCalled)
			require.Equal(t, tt.wantCalled, called)
			if tt.wantBindError {
				var bindErr *fiber.BindError
				require.ErrorAs(t, commandErr, &bindErr)
				require.Equal(t, fiber.BindSourceBody, bindErr.Source)
				var syntaxErr *json.SyntaxError
				require.ErrorAs(t, bindErr, &syntaxErr)
			} else {
				require.ErrorIs(t, commandErr, tt.wantError)
			}
			if tt.wantCalled {
				require.Equal(t, result{Actor: wantActor, Name: "updated"}, got)
			}
		})
	}
}
