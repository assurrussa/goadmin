//go:build integration

package adminservice_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/postgres"
	"github.com/assurrussa/goauth/rbac"
	logger "github.com/assurrussa/gologger"
	"github.com/gofiber/fiber/v3"
	fibersession "github.com/gofiber/fiber/v3/middleware/session"
	redis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	sessioncore "github.com/assurrussa/goadmin/infrastructure/core/session"
	"github.com/assurrussa/goadmin/infrastructure/outbox"
	adminrepo "github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminrepo"
	adminroles "github.com/assurrussa/goadmin/infrastructure/roles/adminroles"
	internalauth "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/auth/browserstate"
	identity "github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
	"github.com/assurrussa/goadmin/services/adminservice"
	"github.com/assurrussa/goadmin/tests"
)

func TestPostgresAdminOpaqueRefreshAndRevocation(t *testing.T) {
	for _, backend := range []string{"pgsql", "redis"} {
		t.Run(backend, func(t *testing.T) {
			ctx := t.Context()
			db, _, cleanup := tests.PrepareDB(ctx, t, "AdminOpaqueAcceptance")
			t.Cleanup(func() { cleanup(context.Background()) })
			repo := adminrepo.Must(adminrepo.NewOptions(db, outbox.PgsqlTrxNew(db.DB())))
			var clock atomic.Int64
			clock.Store(time.Now().UTC().Truncate(time.Second).UnixNano())
			cfg := tests.AuthRuntimeConfig(t)
			cfg.AccessTTL = time.Second
			cfg.Now = func() time.Time { return time.Unix(0, clock.Load()) }
			adapter, err := internalauth.New(internalauth.Config{
				Database: db, Memberships: repo, Runtime: cfg,
				NotificationSender: tests.AuthNotificationSender(),
			})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, adapter.Close()) })
			account, err := adapter.Runtime().ProvisionTrustedLocalAccount(ctx, goauth.RegisterRequest{Email: "admin-acceptance@example.test", Password: pgAdminPassword})
			require.NoError(t, err)
			admin, err := repo.ProvisionAccount(ctx, account, identity.NewUserID())
			require.NoError(t, err)
			roles, err := adapter.Runtime().RBAC(nil)
			require.NoError(t, err)
			permission := rbac.MustPermissionKey("acceptance", "read")
			_, err = roles.UpsertPermission(ctx, rbac.Permission{Key: permission, Description: "Read acceptance"})
			require.NoError(t, err)
			role, err := roles.CreateRole(ctx, rbac.Role{Slug: "acceptance", Name: "Acceptance"}, []rbac.PermissionKey{permission})
			require.NoError(t, err)
			require.NoError(t, roles.ReplaceSubjectRoles(ctx, account.Subject.ID, []int64{role.ID}))
			guard, err := internalauth.NewGuardService(internalauth.MustListSubjectRolesUseCase(adapter.Roles()), adapter.Roles(), logger.Discard())
			require.NoError(t, err)
			facade := adminroles.Must(guard, repo)
			var conn *redis.Client
			if backend == "redis" {
				address := os.Getenv("GOAUTH_TEST_REDIS_ADDRESS")
				require.NotEmpty(t, address)
				conn = redis.NewClient(&redis.Options{Addr: address})
				t.Cleanup(func() { _ = conn.Close() })
			}
			authority, err := adapter.BrowserState(backend, conn)
			require.NoError(t, err)
			states := &pgObservedStateStore{Store: authority, pendingRead: make(chan struct{}, 10)}
			store := fibersession.NewStore()
			r := &pgPausedRefreshRuntime{Runtime: adapter.Runtime(), entered: make(chan struct{}, 1), release: make(chan struct{})}
			one := adminservice.NewService(store, facade, adminservice.WithRuntime(r, repo), adminservice.WithBrowserState(states))
			two := adminservice.NewService(store, facade, adminservice.WithRuntime(r, repo), adminservice.WithBrowserState(states))
			app := fiber.New()
			app.Post("/login", func(c fiber.Ctx) error {
				_, _, err := one.LoginAdmin(c, account.PrimaryEmail.DisplayValue, pgAdminPassword, false)
				if err != nil {
					return err
				}
				return c.SendStatus(204)
			})
			for path, svc := range map[string]*adminservice.Service{"/one": one, "/two": two} {
				app.Get(path, func(c fiber.Ctx) error {
					actor, err := svc.GetAdminAuth(c)
					if err != nil {
						return err
					}
					if actor == nil {
						return c.SendStatus(401)
					}
					if !facade.AdminGuardCheck(c, actor.ID, internalauth.NewPermissionKey("acceptance", "read")) {
						return c.SendStatus(403)
					}
					return c.SendStatus(204)
				})
			}
			app.Post("/rotate", func(c fiber.Ctx) error {
				_, err := one.RotateAdminAuth(c, models.Admin{})
				if err != nil {
					return err
				}
				return c.SendStatus(204)
			})
			login := pgPerformSessionRequest(t, app, http.MethodPost, "/login", nil)
			require.Equal(t, 204, login.StatusCode)
			cookie := pgRequireSessionCookie(t, login)
			require.NoError(t, login.Body.Close())
			t.Cleanup(func() { _ = states.Delete(context.Background(), cookie.Value) })
			stale, err := store.GetByID(ctx, cookie.Value)
			require.NoError(t, err)
			defer stale.Release()
			require.NotNil(t, stale.Get(sessioncore.AuthAdminKey.String()))
			clock.Add(int64(2 * time.Second))
			results := make(chan int, 2)
			failures := make(chan error, 2)
			request := func(path string) {
				req := httptest.NewRequestWithContext(ctx, http.MethodGet, path, nil)
				req.AddCookie(cookie)
				resp, err := app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second})
				if err != nil {
					failures <- err
					return
				}
				defer resp.Body.Close()
				results <- resp.StatusCode
			}
			go request("/one")
			select {
			case <-r.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("refresh did not start")
			}
			<-states.pendingRead
			go request("/two")
			select {
			case <-states.pendingRead:
			case <-time.After(5 * time.Second):
				t.Fatal("second process did not observe claim")
			}
			close(r.release)
			for range 2 {
				select {
				case code := <-results:
					require.Equal(t, 204, code)
				case err := <-failures:
					t.Fatal(err)
				case <-time.After(10 * time.Second):
					t.Fatal("request stalled")
				}
			}
			require.Equal(t, int32(1), r.calls.Load())
			before, found, err := states.Load(ctx, cookie.Value)
			require.NoError(t, err)
			require.True(t, found)
			require.NoError(t, stale.Save())
			after, found, err := states.Load(ctx, cookie.Value)
			require.NoError(t, err)
			require.True(t, found)
			require.Equal(t, before.Tokens.RefreshToken, after.Tokens.RefreshToken)
			require.NoError(t, roles.ReplaceRolePermissions(ctx, role.ID, nil))
			denied := pgPerformSessionRequest(t, app, http.MethodGet, "/one", []*http.Cookie{cookie})
			require.Equal(t, 403, denied.StatusCode)
			_ = denied.Body.Close()
			require.NoError(t, roles.ReplaceRolePermissions(ctx, role.ID, []rbac.PermissionKey{permission}))
			rotated := pgPerformSessionRequest(t, app, http.MethodPost, "/rotate", []*http.Cookie{cookie})
			require.Equal(t, 204, rotated.StatusCode)
			newCookie := pgRequireSessionCookie(t, rotated)
			_ = rotated.Body.Close()
			require.NotEqual(t, cookie.Value, newCookie.Value)
			old := pgPerformSessionRequest(t, app, http.MethodGet, "/one", []*http.Cookie{cookie})
			require.Equal(t, 401, old.StatusCode)
			_ = old.Body.Close()
			current := pgPerformSessionRequest(t, app, http.MethodGet, "/one", []*http.Cookie{newCookie})
			require.Equal(t, 204, current.StatusCode)
			_ = current.Body.Close()
			t.Cleanup(func() { _ = states.Delete(context.Background(), newCookie.Value) })
			_, err = adapter.Runtime().LogoutAll(ctx, account.Subject.ID)
			require.NoError(t, err)
			revoked := pgPerformSessionRequest(t, app, http.MethodGet, "/one", []*http.Cookie{newCookie})
			require.Equal(t, 401, revoked.StatusCode)
			_ = revoked.Body.Close()
			// The same canonical account can log in again after coordinated revocation.
			again := pgPerformSessionRequest(t, app, http.MethodPost, "/login", nil)
			require.Equal(t, 204, again.StatusCode)
			active := pgRequireSessionCookie(t, again)
			_ = again.Body.Close()
			_, err = db.DB().Pool().Exec(ctx, "UPDATE administrations SET deleted_at=now() WHERE id=$1", admin.ID)
			require.NoError(t, err)
			removed := pgPerformSessionRequest(t, app, http.MethodGet, "/one", []*http.Cookie{active})
			require.Equal(t, 401, removed.StatusCode)
			_ = removed.Body.Close()
		})
	}
}

const pgAdminPassword = "Admin-Unique-Passphrase-123" //nolint:gosec // isolated fixture only.
type pgPausedRefreshRuntime struct {
	*postgres.Runtime
	calls            atomic.Int32
	entered, release chan struct{}
}

func (r *pgPausedRefreshRuntime) Refresh(ctx context.Context, token string) (goauth.TokenPair, error) {
	r.calls.Add(1)
	r.entered <- struct{}{}
	<-r.release
	return r.Runtime.Refresh(ctx, token)
}

type pgObservedStateStore struct {
	browserstate.Store
	pendingRead chan struct{}
}

func (s *pgObservedStateStore) Load(ctx context.Context, id string) (browserstate.Record, bool, error) {
	r, ok, err := s.Store.Load(ctx, id)
	if r.Owner != "" {
		select {
		case s.pendingRead <- struct{}{}:
		default:
		}
	}
	return r, ok, err
}

func pgPerformSessionRequest(t *testing.T, app *fiber.App, method, path string, cookies []*http.Cookie) *http.Response {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second})
	require.NoError(t, err)
	return resp
}

func pgRequireSessionCookie(t *testing.T, resp *http.Response) *http.Cookie {
	t.Helper()
	for _, c := range resp.Cookies() {
		if c.Name == "session_id" {
			return c
		}
	}
	t.Fatal("session cookie missing")
	return nil
}
