//nolint:testpackage // exercises the real owner/follower transition with injected Runtime outcomes.
package adminservice

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/internal/auth/browserstate"
	"github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
)

type retryObservedJournal struct {
	browserstate.Store
	followerClaimed chan struct{}
	once            sync.Once
}

func (s *retryObservedJournal) Claim(ctx context.Context, id string, version int64, owner string) (bool, error) {
	claimed, err := s.Store.Claim(ctx, id, version, owner)
	if err == nil && !claimed {
		s.once.Do(func() { close(s.followerClaimed) })
	}
	return claimed, err
}

type concurrentRefreshRouteResult struct {
	response *http.Response
	err      error
}

func setupConcurrentRefreshApp(app *fiber.App, one, two *Service, temporary error, mutations *atomic.Int32) {
	for route, svc := range map[string]*Service{"/leader": one, "/follower": two, "/retry": two} {
		app.Post(route, func(c fiber.Ctx) error {
			actor, err := svc.GetAdminAuth(c)
			if errors.Is(err, temporary) || errors.Is(err, ErrRefreshRetryRequired) {
				return c.SendStatus(http.StatusServiceUnavailable)
			}
			if err != nil || actor == nil {
				return c.SendStatus(http.StatusUnauthorized)
			}
			mutations.Add(1)
			return c.SendStatus(http.StatusNoContent)
		})
	}
}

func verifyConcurrentRefreshRollbackResponses(t *testing.T, ctx context.Context, results <-chan concurrentRefreshRouteResult) {
	t.Helper()
	for range 2 {
		select {
		case got := <-results:
			require.NoError(t, got.err)
			require.Equal(t, http.StatusServiceUnavailable, got.response.StatusCode)
			for _, returned := range got.response.Cookies() {
				require.NotEqual(t, -1, returned.MaxAge, "retryable failure erased a browser cookie")
			}
			require.NoError(t, got.response.Body.Close())
		case <-ctx.Done():
			t.Fatal("refresh requests did not finish")
		}
	}
}

func runConcurrentRefreshRollback(t *testing.T, accessErr error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	store := session.NewStore()
	app, cookie := seedAdminBrowserSession(t, store)
	initial := loadBrowserSession(t, store, cookie.Value)
	journal := &retryObservedJournal{
		Store: browserstate.NewMemory(), followerClaimed: make(chan struct{}),
	}
	//nolint:gosec // synthetic test tokens
	tokens := goauth.TokenPair{AccessToken: testOldAccessToken, RefreshToken: "unconsumed-refresh"}
	tokens.Session.ID = testCanonicalSession
	tokens.Session.ExpiresAt = time.Now().Add(time.Hour)
	require.NoError(t, journal.Create(ctx, cookie.Value, browserstate.Record{
		SubjectID: initial.SubjectID, Tokens: tokens,
	}, time.Hour))

	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseLeader := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseLeader()
	temporary := errors.New("deterministic pre-consumption backend failure")
	var refreshes, mutations atomic.Int32
	rt := &runtimeStub{
		verify: func(_ context.Context, token string, _ bool) (goauth.AuthContext, error) {
			if token != testNewAccessToken {
				return goauth.AuthContext{}, accessErr
			}
			return goauth.AuthContext{Realm: goauth.RealmAdmin, SubjectID: initial.SubjectID}, nil
		},
		refresh: func(ctx context.Context, token string) (goauth.TokenPair, error) {
			n := refreshes.Add(1)
			if token != tokens.RefreshToken {
				return goauth.TokenPair{}, goauth.ErrInvalidToken
			}
			if n == 1 {
				close(entered)
				select {
				case <-release:
					return goauth.TokenPair{}, temporary
				case <-ctx.Done():
					return goauth.TokenPair{}, ctx.Err()
				}
			}
			next := tokens
			next.AccessToken, next.RefreshToken = testNewAccessToken, testNewRefreshToken
			return next, nil
		},
		getAccount: func(context.Context, goauth.SubjectID) (goauth.Account, error) {
			return goauth.Account{}, nil
		},
	}
	admin := models.Admin{ID: 1, SubjectID: initial.SubjectID, UUID: identity.NewUserID()}
	one := NewService(store, roleServiceStub{}, WithRuntime(rt, adminRepositoryStub{admin: admin}), WithBrowserState(journal))
	two := NewService(store, roleServiceStub{}, WithRuntime(rt, adminRepositoryStub{admin: admin}), WithBrowserState(journal))
	setupConcurrentRefreshApp(app, one, two, temporary, &mutations)
	request := func(route string) concurrentRefreshRouteResult {
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, route, nil)
		req.AddCookie(cookie)
		//nolint:bodyclose // closed by receiver
		response, err := app.Test(req, fiber.TestConfig{
			Timeout: 10 * time.Second,
		})
		return concurrentRefreshRouteResult{response, err}
	}
	results := make(chan concurrentRefreshRouteResult, 2)
	go func() { results <- request("/leader") }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("leader did not enter refresh")
	}
	go func() { results <- request("/follower") }()
	select {
	case <-journal.followerClaimed:
	case <-ctx.Done():
		t.Fatal("follower did not lose the CAS claim")
	}
	releaseLeader()
	verifyConcurrentRefreshRollbackResponses(t, ctx, results)
	require.EqualValues(t, 1, refreshes.Load(), "follower must not recursively consume the refresh token")
	require.Zero(t, mutations.Load(), "failed requests must not run the mutation")
	record, found, err := journal.Load(ctx, cookie.Value)
	require.NoError(t, err)
	require.True(t, found)
	require.Empty(t, record.Owner)
	require.EqualValues(t, 2, record.Version)
	require.Equal(t, tokens.RefreshToken, record.Tokens.RefreshToken)

	// A later explicit request acquires a new claim and consumes the still
	// valid refresh exactly once. No failed mutation is replayed for it.
	got := request("/retry")
	require.NoError(t, got.err)
	require.Equal(t, http.StatusNoContent, got.response.StatusCode)
	require.NoError(t, got.response.Body.Close())
	require.EqualValues(t, 2, refreshes.Load())
	require.EqualValues(t, 1, mutations.Load())
	record, found, err = journal.Load(ctx, cookie.Value)
	require.NoError(t, err)
	require.True(t, found)
	require.EqualValues(t, 3, record.Version)
	require.Equal(t, testNewRefreshToken, record.Tokens.RefreshToken)
}

func TestConcurrentRefreshRollbackPreservesAuthority(t *testing.T) {
	for _, accessErr := range []error{goauth.ErrExpiredToken, goauth.ErrInvalidToken} {
		t.Run(accessErr.Error(), func(t *testing.T) {
			runConcurrentRefreshRollback(t, accessErr)
		})
	}
}
