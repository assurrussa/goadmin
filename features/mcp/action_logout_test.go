package mcp //nolint:testpackage // Verify native logout and grant retirement through the private middleware.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/testkit"
	pgstore "github.com/assurrussa/outbox/backends/pgsql/storage"
	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/assurrussa/goadmin/adminapp/adminappt"
	"github.com/assurrussa/goadmin/host"
	authhandler "github.com/assurrussa/goadmin/http/handlers/auth"
	"github.com/assurrussa/goadmin/internal/auth/browserstate"
	"github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
	"github.com/assurrussa/goadmin/services/adminservice"
)

func TestConfirmedActionNativeLogoutRevokesGrant(t *testing.T) {
	for _, tc := range []struct {
		name       string
		cleanupErr error
		wantStatus int
	}{
		{name: "cleanup succeeds", wantStatus: http.StatusFound},
		{name: "cleanup fails", cleanupErr: errors.New("fixture cleanup failed"), wantStatus: http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const loginPath, logoutPath = "/auth/login", "/auth/logout"
			const password = "Native-Logout-Test-Passphrase-42" //nolint:gosec // Synthetic account in the in-memory fixture.
			canonical, err := testkit.NewRuntime()
			require.NoError(t, err)
			account, err := canonical.Runtime.ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
				Email: "mcp-logout@example.test", Password: password,
			})
			require.NoError(t, err)
			admin := models.Admin{ID: 42, SubjectID: account.Subject.ID, UUID: identity.NewUserID()}
			fixture := adminappt.NewAppTest(t)
			fixture.MockRoleService.EXPECT().GetRolesAdmin(gomock.Any(), admin.ID).Return(nil, nil).AnyTimes()
			states := browserstate.NewMemory()
			service := adminservice.NewService(fixture.SessionStore, fixture.MockRoleService,
				adminservice.WithRuntime(canonical.Runtime, actionLogoutAdminRepository{admin: admin}),
				adminservice.WithBrowserState(states))
			h := &actionHandler{
				sessionName: fixture.SessionStore.Extractor.Key,
				grants:      newActionPairings(),
				store: actionStore{
					tx:     fixture.MockTransaction,
					action: Action{Namespace: "logout-test", Key: "cleanup"},
				},
			}
			app := fiber.New()
			app.Use(h.nativeTransitions)
			app.Post(loginPath, func(c fiber.Ctx) error {
				if _, _, err := service.LoginAdmin(c, account.PrimaryEmail.DisplayValue, password, false); err != nil {
					return err
				}
				return c.SendStatus(http.StatusNoContent)
			})
			auth := authhandler.NewHandler(authhandler.HandlerOptions{AdminApp: fixture.App, SessionStoreActions: service})
			app.Delete(logoutPath, auth.Logout)

			login, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodPost, loginPath, nil))
			require.NoError(t, err)
			require.NoError(t, login.Body.Close())
			require.Equal(t, http.StatusNoContent, login.StatusCode)
			cookie := actionLogoutSessionCookie(t, login, h.sessionName)
			require.NotEmpty(t, cookie.Value)
			state, found, err := states.Load(t.Context(), cookie.Value)
			require.NoError(t, err)
			require.True(t, found)
			before, err := canonical.Store.IntrospectSession(t.Context(), state.Tokens.Session.ID)
			require.NoError(t, err)
			require.Nil(t, before.Session.RevokedAt)

			code, secret, started := h.grants.start(netip.MustParseAddr("127.0.0.1"))
			require.True(t, started)
			require.True(t, h.grants.approve(code, actionGrant{
				actor:   host.Actor{AdminID: admin.ID, SubjectID: admin.SubjectID.String()},
				browser: host.CSRFSessionBinding(cookie.Value), deadline: time.Now().Add(time.Minute),
			}))
			grant, token, _, status := h.grants.exchange(secret)
			require.Equal(t, http.StatusOK, status)
			cleanup := &actionLogoutTx{err: tc.cleanupErr}
			fixture.MockTransaction.EXPECT().RunInTx(gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, run func(context.Context) error) error {
					return run(pgstore.WithTx(ctx, cleanup))
				})

			request := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, logoutPath, nil)
			request.AddCookie(cookie)
			logout, err := app.Test(request)
			require.NoError(t, err)
			defer logout.Body.Close()
			body, err := io.ReadAll(logout.Body)
			require.NoError(t, err)
			require.Equal(t, tc.wantStatus, logout.StatusCode, string(body))
			if tc.cleanupErr != nil {
				require.JSONEq(t, `{"error":"unavailable"}`, string(body))
			}
			require.Equal(t, loginPath, logout.Header.Get("Location"))
			cleared := actionLogoutSessionCookie(t, logout, h.sessionName)
			require.Empty(t, cleared.Value)
			require.Negative(t, cleared.MaxAge)
			require.Equal(t, []any{h.store.action.Namespace, h.store.action.Key, []int64{admin.ID}, []string{grant.id}}, cleanup.args)
			_, active := h.grants.bearer(token)
			require.False(t, active)
			_, bound := h.grants.bound(grant.id)
			require.False(t, bound)
			_, found, err = states.Load(t.Context(), cookie.Value)
			require.NoError(t, err)
			require.False(t, found)
			after, err := canonical.Store.IntrospectSession(t.Context(), state.Tokens.Session.ID)
			require.NoError(t, err)
			require.NotNil(t, after.Session.RevokedAt)
		})
	}
}

func actionLogoutSessionCookie(t *testing.T, response *http.Response, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("response is missing %q cookie", name)
	return nil
}

type actionLogoutAdminRepository struct{ admin models.Admin }

func (r actionLogoutAdminRepository) GetBySubjectID(_ context.Context, subject goauth.SubjectID) (models.Admin, error) {
	if subject == r.admin.SubjectID {
		return r.admin, nil
	}
	return models.Admin{}, errors.New("fixture admin membership not found")
}

type actionLogoutTx struct {
	pgx.Tx
	args []any
	err  error
}

func (tx *actionLogoutTx) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	tx.args = args
	return pgconn.NewCommandTag("UPDATE 1"), tx.err
}
