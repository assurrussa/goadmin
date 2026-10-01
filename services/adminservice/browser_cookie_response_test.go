//nolint:testpackage // verifies private session transitions and browser cookie delivery order.
package adminservice

import (
	"context"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/testkit"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/require"

	sessioncore "github.com/assurrussa/goadmin/infrastructure/core/session"
	"github.com/assurrussa/goadmin/internal/auth/browserstate"
	"github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/internal/refreshpolicy"
	"github.com/assurrussa/goadmin/models"
)

const (
	readerAuthority = "authority"
	readerWaiter    = "waiter"
)

type pausedCookieJournal struct {
	browserstate.Store
	armed   atomic.Bool
	entered chan struct{}
	resume  chan struct{}
}

func (s *pausedCookieJournal) Load(ctx context.Context, id string) (browserstate.Record, bool, error) {
	if s.armed.Swap(false) {
		close(s.entered)
		select {
		case <-s.resume:
		case <-ctx.Done():
			return browserstate.Record{}, false, ctx.Err()
		}
	}
	return s.Store.Load(ctx, id)
}

func setupStaleCookieOrderApp(
	svc *Service, store *session.Store, admin models.Admin, account goauth.Account, reader string, staleMutations *atomic.Int32,
) *fiber.App {
	app := fiber.New()
	app.Post("/login", func(c fiber.Ctx) error {
		_, _, err := svc.LoginAdmin(c, account.PrimaryEmail.DisplayValue, testAdminPassword, true)
		if err != nil {
			return err
		}
		return c.SendStatus(http.StatusNoContent)
	})
	app.Post("/old", func(c fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}
		defer sess.Release()
		state, ok := sess.Get(sessioncore.AuthAdminKey.String()).(*browserSession)
		if !ok || state == nil {
			return errors.New("missing test browser session")
		}
		// Reproduce a cookie already queued by an earlier session user.
		sess.Set("flash", "older request")
		if err := sess.Save(); err != nil {
			return err
		}
		svc.deletePolicyCookie(c)
		c.Cookie(&fiber.Cookie{Name: "unrelated", Value: "preserved", Path: "/"})
		var active bool
		if reader == readerAuthority {
			actor, readErr := svc.GetAdminAuth(c)
			active, err = actor != nil, readErr
		} else {
			actor, readErr := svc.waitAdminRefresh(c, sess, state)
			active, err = actor != nil, readErr
		}
		if err != nil {
			return err
		}
		if !active {
			return c.SendStatus(http.StatusUnauthorized)
		}
		staleMutations.Add(1)
		return c.SendStatus(http.StatusNoContent)
	})
	app.Post("/rotate", func(c fiber.Ctx) error {
		id, err := svc.RotateAdminAuth(c, admin)
		if err != nil {
			return err
		}
		// Use the ID returned by this explicit transition, not the old
		// request cookie when applying its remember-me policy.
		svc.writeSessionCookie(c, id, true)
		return c.SendStatus(http.StatusNoContent)
	})
	app.Get("/current", func(c fiber.Ctx) error {
		actor, err := svc.GetAdminAuth(c)
		if err != nil {
			return err
		}
		if actor == nil {
			return c.SendStatus(http.StatusUnauthorized)
		}
		return c.SendStatus(http.StatusNoContent)
	})
	app.Post("/logout", func(c fiber.Ctx) error {
		if err := svc.DelAdminAuth(c); err != nil {
			return err
		}
		return c.SendStatus(http.StatusNoContent)
	})
	return app
}

type staleCookieTestResult struct {
	response *http.Response
	err      error
}

func runStaleCookieOrderTest(t *testing.T, reader string, newFirst bool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	fixture, err := testkit.NewRuntime()
	require.NoError(t, err)
	account, err := fixture.Runtime.ProvisionTrustedLocalAccount(ctx, goauth.RegisterRequest{
		Email: "cookie-order@example.test", Password: testAdminPassword,
	})
	require.NoError(t, err)
	admin := models.Admin{ID: 1, SubjectID: account.Subject.ID, UUID: identity.NewUserID()}
	store := session.NewStore()
	journal := &pausedCookieJournal{
		Store: browserstate.NewMemory(), entered: make(chan struct{}), resume: make(chan struct{}),
	}
	var resumeOnce sync.Once
	resume := func() { resumeOnce.Do(func() { close(journal.resume) }) }
	defer resume()
	svc := NewService(
		store,
		roleServiceStub{},
		WithRuntime(fixture.Runtime, adminRepositoryStub{admin: admin}),
		WithBrowserState(journal),
	)
	var staleMutations atomic.Int32
	app := setupStaleCookieOrderApp(svc, store, admin, account, reader, &staleMutations)
	request := func(method, path string, cookies []*http.Cookie) (*http.Response, error) {
		req := httptest.NewRequestWithContext(ctx, method, path, nil)
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		return app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second})
	}
	login, err := request(http.MethodPost, "/login", nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, login.StatusCode)
	require.NoError(t, login.Body.Close())
	oldCookie := requireSessionCookie(t, login)
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	origin, err := url.Parse("http://admin.example.test/")
	require.NoError(t, err)
	jar.SetCookies(origin, login.Cookies())
	oldCookies := jar.Cookies(origin)
	finished := make(chan staleCookieTestResult, 1)
	journal.armed.Store(true)
	go func() {
		resp, reqErr := request(http.MethodPost, "/old", oldCookies) //nolint:bodyclose // closed by receiving test flow
		finished <- staleCookieTestResult{resp, reqErr}
	}()
	select {
	case <-journal.entered:
	case <-ctx.Done():
		t.Fatal("old request did not reach the journal")
	}
	rotated, err := request(http.MethodPost, "/rotate", oldCookies)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, rotated.StatusCode)
	require.NoError(t, rotated.Body.Close())
	newCookie := requireSessionCookie(t, rotated)
	require.NotEqual(t, oldCookie.Value, newCookie.Value)
	resume()
	var stale *http.Response
	select {
	case got := <-finished:
		require.NoError(t, got.err)
		stale = got.response
	case <-ctx.Done():
		t.Fatal("old request did not finish")
	}
	require.Equal(t, http.StatusUnauthorized, stale.StatusCode)
	require.NoError(t, stale.Body.Close())
	for _, cookie := range stale.Cookies() {
		require.NotEqual(t, svc.sessionCookieName(), cookie.Name)
		require.NotEqual(t, svc.policyCookieName(), cookie.Name)
	}
	require.Zero(t, staleMutations.Load())
	ordered := []*http.Response{stale, rotated}
	if newFirst {
		ordered = []*http.Response{rotated, stale}
	}
	for _, response := range ordered {
		jar.SetCookies(origin, response.Cookies())
	}
	values := make(map[string]string)
	for _, cookie := range jar.Cookies(origin) {
		values[cookie.Name] = cookie.Value
	}
	require.Equal(t, newCookie.Value, values[svc.sessionCookieName()])
	require.Equal(t, "1", values[svc.policyCookieName()])
	require.Equal(t, "preserved", values["unrelated"])
	current, err := request(http.MethodGet, "/current", jar.Cookies(origin))
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, current.StatusCode)
	require.NoError(t, current.Body.Close())
	// The passive-response change does not disable explicit logout.
	logout, err := request(http.MethodPost, "/logout", jar.Cookies(origin))
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, logout.StatusCode)
	require.Negative(t, requireSessionCookie(t, logout).MaxAge)
	require.NoError(t, logout.Body.Close())
	_, found, err := journal.Load(ctx, newCookie.Value)
	require.NoError(t, err)
	require.False(t, found)
}

func TestStaleResponsePreservesRotatedCookieInBothDeliveryOrders(t *testing.T) {
	for _, reader := range []string{readerAuthority, readerWaiter} {
		for _, newFirst := range []bool{false, true} {
			name := reader + "/old-response-first"
			if newFirst {
				name = reader + "/new-response-first"
			}
			t.Run(name, func(t *testing.T) {
				runStaleCookieOrderTest(t, reader, newFirst)
			})
		}
	}
}

func testFencedAuthorityCase(t *testing.T, kind, reader string) {
	t.Helper()

	store := session.NewStore()
	app, cookie := seedAdminBrowserSession(t, store)
	state := loadBrowserSession(t, store, cookie.Value)
	journal := browserstate.NewMemory()
	require.NoError(t, journal.Create(t.Context(), cookie.Value, browserstate.Record{SubjectID: state.SubjectID}, time.Hour))
	owner := refreshpolicy.Owner("rotate", "test", time.Now())
	if kind == "abandoned" {
		owner = refreshpolicy.Owner("refresh", "test", time.Now().Add(-time.Minute))
	}
	claimed, err := journal.Claim(t.Context(), cookie.Value, 1, owner)
	require.NoError(t, err)
	require.True(t, claimed)
	svc := NewService(store, roleServiceStub{}, WithBrowserState(journal))
	app.Get("/fenced", func(c fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return err
		}
		defer sess.Release()
		svc.writeSessionCookie(c, sess.ID(), true)
		if reader == readerAuthority {
			active, err := svc.loadBrowserAuthority(c, sess, &state)
			if err != nil || active {
				return fiber.ErrInternalServerError
			}
		} else {
			actor, err := svc.waitAdminRefresh(c, sess, &state)
			if err != nil || actor != nil {
				return fiber.ErrInternalServerError
			}
		}
		return c.SendStatus(http.StatusNoContent)
	})
	response := performSessionRequest(t, app, http.MethodGet, "/fenced", cookie)
	require.Empty(t, response.Cookies())
	require.NoError(t, response.Body.Close())
	retained, found, err := journal.Load(t.Context(), cookie.Value)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, owner, retained.Owner)
	claimed, err = journal.Claim(t.Context(), cookie.Value, 1, "another-owner")
	require.NoError(t, err)
	require.False(t, claimed, "a fenced refresh secret must remain unclaimable")
}

func TestFencedAuthorityRejectsWithoutChangingCookiesOrUnlocking(t *testing.T) {
	for _, kind := range []string{"rotate", "abandoned"} {
		for _, reader := range []string{readerAuthority, readerWaiter} {
			t.Run(kind+"/"+reader, func(t *testing.T) {
				testFencedAuthorityCase(t, kind, reader)
			})
		}
	}
}
