//nolint:testpackage // verifies private follower state transitions.
package adminservice

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/internal/auth/browserstate"
)

func TestRefreshFollowerClearsTerminalCanonicalFailure(t *testing.T) {
	for _, terminal := range []error{goauth.ErrMembershipDenied, goauth.ErrSessionRevoked, goauth.ErrSecurityVersionMismatch} {
		t.Run(terminal.Error(), func(t *testing.T) {
			store := session.NewStore()
			app, cookie := seedAdminBrowserSession(t, store)
			state := loadBrowserSession(t, store, cookie.Value)
			state.Version = 1
			journal := browserstate.NewMemory()
			record := browserstate.Record{
				SubjectID: state.SubjectID,
				Tokens:    goauth.TokenPair{AccessToken: "committed-access", RefreshToken: "committed-refresh"},
			}
			require.NoError(t, journal.Create(t.Context(), cookie.Value, record, time.Hour))
			claimed, err := journal.Claim(t.Context(), cookie.Value, 1, "test-winner")
			require.NoError(t, err)
			require.True(t, claimed)
			completed, err := journal.Complete(t.Context(), cookie.Value, 1, "test-winner", record)
			require.NoError(t, err)
			require.True(t, completed)
			svc := NewService(store, roleServiceStub{}, WithRuntime(&runtimeStub{
				verify: func(context.Context, string, bool) (goauth.AuthContext, error) {
					return goauth.AuthContext{}, terminal
				},
			}, adminRepositoryStub{}), WithBrowserState(journal))
			app.Get("/follower", func(c fiber.Ctx) error {
				sess, err := store.Get(c)
				if err != nil {
					return err
				}
				defer sess.Release()
				actor, err := svc.waitAdminRefresh(c, sess, &state)
				if err != nil || actor != nil {
					return c.SendStatus(500)
				}
				return c.SendStatus(204)
			})
			response := performSessionRequest(t, app, http.MethodGet, "/follower", cookie)
			require.Equal(t, 204, response.StatusCode)
			require.Negative(t, requireSessionCookie(t, response).MaxAge)
			require.NoError(t, response.Body.Close())
			_, found, err := journal.Load(t.Context(), cookie.Value)
			require.NoError(t, err)
			require.False(t, found)
		})
	}
}
