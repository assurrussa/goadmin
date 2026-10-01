//nolint:testpackage,lll // Tests exercise private security state and keep fixture contracts together.
package adminservice

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/testkit"
	"github.com/gofiber/fiber/v3"
	fibersession "github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/require"

	sessioncore "github.com/assurrussa/goadmin/infrastructure/core/session"
	"github.com/assurrussa/goadmin/internal/auth/browserstate"
	identity "github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
)

type pausedRefreshRuntime struct {
	runtime
	calls            atomic.Int32
	entered, release chan struct{}
}

func (r *pausedRefreshRuntime) Refresh(ctx context.Context, token string) (goauth.TokenPair, error) {
	r.calls.Add(1)
	r.entered <- struct{}{}
	<-r.release
	return r.runtime.Refresh(ctx, token)
}

type observedStateStore struct {
	browserstate.Store
	pendingRead chan struct{}
}

func (s *observedStateStore) Load(ctx context.Context, id string) (browserstate.Record, bool, error) {
	r, ok, err := s.Store.Load(ctx, id)
	if r.Owner != "" {
		select {
		case s.pendingRead <- struct{}{}:
		default:
		}
	}
	return r, ok, err
}

func TestConcurrentAdminRefreshAcrossInstancesAndStaleSessionSave(t *testing.T) {
	var clock atomic.Int64
	clock.Store(time.Now().UTC().Truncate(time.Second).UnixNano())
	f, err := testkit.NewRuntime(func(c *goauth.Config) {
		c.AccessTTL = time.Second
		c.Now = func() time.Time { return time.Unix(0, clock.Load()) }
	})
	require.NoError(t, err)
	account, err := f.Runtime.ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{Email: "admin-race@example.test", Password: testAdminPassword})
	require.NoError(t, err)
	admin := models.Admin{ID: 91, SubjectID: account.Subject.ID, UUID: identity.NewUserID()}
	store := fibersession.NewStore()
	states := &observedStateStore{Store: browserstate.NewMemory(), pendingRead: make(chan struct{}, 10)}
	r := &pausedRefreshRuntime{runtime: f.Runtime, entered: make(chan struct{}, 1), release: make(chan struct{})}
	s1 := NewService(store, roleServiceStub{}, WithRuntime(r, adminRepositoryStub{admin: admin}), WithBrowserState(states))
	s2 := NewService(store, roleServiceStub{}, WithRuntime(r, adminRepositoryStub{admin: admin}), WithBrowserState(states))
	app := fiber.New()
	app.Post("/login", func(c fiber.Ctx) error {
		_, _, err := s1.LoginAdmin(c, account.PrimaryEmail.DisplayValue, testAdminPassword, false)
		if err != nil {
			return err
		}
		return c.SendStatus(204)
	})
	for path, svc := range map[string]*Service{"/a": s1, "/b": s2} {
		app.Get(path, func(c fiber.Ctx) error {
			actor, err := svc.GetAdminAuth(c)
			if err != nil {
				return err
			}
			if actor == nil {
				return c.SendStatus(401)
			}
			return c.SendStatus(204)
		})
	}
	login := performSessionRequest(t, app, http.MethodPost, "/login", nil)
	cookie := requireSessionCookie(t, login)
	require.NoError(t, login.Body.Close())
	stale, err := store.GetByID(t.Context(), cookie.Value)
	require.NoError(t, err)
	defer stale.Release()
	clock.Add(int64(2 * time.Second))
	responses := make(chan int, 2)
	failures := make(chan error, 2)
	request := func(path string) {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		req.AddCookie(cookie)
		resp, err := app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second})
		if err != nil {
			failures <- err
			return
		}
		defer resp.Body.Close()
		responses <- resp.StatusCode
	}
	go request("/a")
	select {
	case <-r.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first refresh did not enter")
	}
	// Consume the winner's load, then observe the independently loaded loser.
	<-states.pendingRead
	go request("/b")
	select {
	case <-states.pendingRead:
	case <-time.After(5 * time.Second):
		t.Fatal("second node did not read pending state")
	}
	close(r.release)
	for range 2 {
		select {
		case code := <-responses:
			require.Equal(t, 204, code)
		case err := <-failures:
			t.Fatal(err)
		case <-time.After(10 * time.Second):
			t.Fatal("request stalled")
		}
	}
	require.Equal(t, int32(1), r.calls.Load(), "same refresh was submitted twice")
	current, ok, err := states.Load(t.Context(), cookie.Value)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, int64(2), current.Version)
	// A flash/avatar request loaded before rotation can still save its complete
	// Fiber snapshot. It must never become credential authority again.
	stale.Set("flash", "late write")
	require.NoError(t, stale.Save())
	response := performSessionRequest(t, app, http.MethodGet, "/b", cookie)
	require.NoError(t, response.Body.Close())
	require.Equal(t, int32(1), r.calls.Load())
	_, err = f.Runtime.LogoutAll(t.Context(), account.Subject.ID)
	require.NoError(t, err)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/a", nil)
	req.AddCookie(cookie)
	response, err = app.Test(req)
	require.NoError(t, err)
	require.Equal(t, 401, response.StatusCode)
	require.NoError(t, response.Body.Close())
	_, ok, err = states.Load(t.Context(), cookie.Value)
	require.NoError(t, err)
	require.False(t, ok)
	old, ok := stale.Get(sessioncore.AuthAdminKey.String()).(*browserSession)
	require.True(t, ok)
	require.NotNil(t, old)
}
