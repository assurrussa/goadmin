//nolint:testpackage // Exercises canonical required-policy denial of a legacy browser projection.
package adminservice

import (
	"context"
	"net/http"
	"testing"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	fibersession "github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/require"

	sessioncore "github.com/assurrussa/goadmin/infrastructure/core/session"
)

func TestRequiredPolicyEvictsLegacySessionAndAllowsReauthentication(t *testing.T) {
	for _, onRefresh := range []bool{false, true} {
		t.Run(map[bool]string{false: "introspection", true: "refresh"}[onRefresh], func(t *testing.T) {
			store := fibersession.NewStore()
			app, cookie := seedAdminBrowserSession(t, store)
			rt := &runtimeStub{
				verify: func(context.Context, string, bool) (goauth.AuthContext, error) {
					if onRefresh {
						return goauth.AuthContext{}, goauth.ErrExpiredToken
					}
					return goauth.AuthContext{}, goauth.ErrMFARequired
				},
				refresh: func(context.Context, string) (goauth.TokenPair, error) {
					require.True(t, onRefresh)
					return goauth.TokenPair{}, goauth.ErrMFARequired
				},
			}
			service := NewService(store, roleServiceStub{}, WithRuntime(rt, adminRepositoryStub{}))
			app.Get("/inspect", func(c fiber.Ctx) error {
				admin, err := service.GetAdminAuth(c)
				require.NoError(t, err)
				require.Nil(t, admin)
				return c.SendStatus(http.StatusNoContent)
			})
			response := performSessionRequest(t, app, http.MethodGet, "/inspect", cookie)
			require.Equal(t, http.StatusNoContent, response.StatusCode)
			require.NoError(t, response.Body.Close())
			sess, err := store.GetByID(t.Context(), cookie.Value)
			require.NoError(t, err)
			defer sess.Release()
			require.Nil(t, sess.Get(sessioncore.AuthAdminKey.String()))
		})
	}
}
