//nolint:testpackage // exercises the private refresh ownership boundary.
package adminservice

import (
	"context"
	"errors"
	"net/http"
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

var errInjectedJournal = errors.New("injected journal failure")

type failingRefreshJournal struct {
	browserstate.Store
	failClaim atomic.Bool
	failLoad  atomic.Bool
}

func (s *failingRefreshJournal) Claim(ctx context.Context, id string, version int64, owner string) (bool, error) {
	ok, err := s.Store.Claim(ctx, id, version, owner)
	if err == nil && ok && s.failClaim.Swap(false) {
		return false, errInjectedJournal // claim committed, reply was lost
	}
	return ok, err
}

func (s *failingRefreshJournal) Load(ctx context.Context, id string) (browserstate.Record, bool, error) {
	record, found, err := s.Store.Load(ctx, id)
	if err == nil && found && record.Owner != "" && s.failLoad.Swap(false) {
		return browserstate.Record{}, false, errInjectedJournal
	}
	return record, found, err
}

func TestRefreshPreConsumptionFailureCanRecoverWithoutConsumingTwice(t *testing.T) {
	for _, mode := range []string{"lost-claim-reply", "claimed-read-failed"} {
		t.Run(mode, func(t *testing.T) {
			store := session.NewStore()
			app, cookie := seedAdminBrowserSession(t, store)
			initial := loadBrowserSession(t, store, cookie.Value)
			journal := &failingRefreshJournal{Store: browserstate.NewMemory()}
			journal.failClaim.Store(mode == "lost-claim-reply")
			journal.failLoad.Store(mode == "claimed-read-failed")
			tokens := goauth.TokenPair{AccessToken: testOldAccessToken, RefreshToken: testOldRefreshToken}
			tokens.Session.ID = testCanonicalSession
			require.NoError(t, journal.Create(
				t.Context(), cookie.Value, browserstate.Record{SubjectID: initial.SubjectID, Tokens: tokens}, time.Hour,
			))
			var refreshes atomic.Int32
			runtime := &runtimeStub{
				verify: func(_ context.Context, token string, _ bool) (goauth.AuthContext, error) {
					if token != testNewAccessToken {
						return goauth.AuthContext{}, goauth.ErrExpiredToken
					}
					return goauth.AuthContext{SubjectID: initial.SubjectID, Realm: goauth.RealmAdmin}, nil
				},
				refresh: func(_ context.Context, token string) (goauth.TokenPair, error) {
					refreshes.Add(1)
					if token != testOldRefreshToken {
						return goauth.TokenPair{}, goauth.ErrInvalidToken
					}
					next := tokens
					next.AccessToken, next.RefreshToken = testNewAccessToken, testNewRefreshToken
					return next, nil
				},
				getAccount: func(context.Context, goauth.SubjectID) (goauth.Account, error) { return goauth.Account{}, nil },
			}
			admin := models.Admin{ID: 1, SubjectID: initial.SubjectID, UUID: identity.NewUserID()}
			svc := NewService(store, roleServiceStub{}, WithRuntime(runtime, adminRepositoryStub{admin: admin}), WithBrowserState(journal))
			app.Get("/fault", func(c fiber.Ctx) error {
				_, err := svc.GetAdminAuth(c)
				if !errors.Is(err, errInjectedJournal) {
					return c.SendStatus(500)
				}
				return c.SendStatus(204)
			})
			response := performSessionRequest(t, app, http.MethodGet, "/fault", cookie)
			require.Equal(t, 204, response.StatusCode)
			require.NoError(t, response.Body.Close())
			require.Zero(t, refreshes.Load())
			record, found, err := journal.Load(t.Context(), cookie.Value)
			require.NoError(t, err)
			require.True(t, found)
			require.Empty(t, record.Owner)
			require.EqualValues(t, 1, record.Version)
			app.Get("/retry", func(c fiber.Ctx) error {
				actor, err := svc.GetAdminAuth(c)
				if err != nil || actor == nil {
					return c.SendStatus(500)
				}
				return c.SendStatus(204)
			})
			response = performSessionRequest(t, app, http.MethodGet, "/retry", cookie)
			require.Equal(t, 204, response.StatusCode)
			require.NoError(t, response.Body.Close())
			require.EqualValues(t, 1, refreshes.Load())
		})
	}
}

func TestRefreshUnknownOutcomeNeverUnlocksOldSecret(t *testing.T) {
	store := session.NewStore()
	app, cookie := seedAdminBrowserSession(t, store)
	initial := loadBrowserSession(t, store, cookie.Value)
	journal := browserstate.NewMemory()
	require.NoError(t, journal.Create(t.Context(), cookie.Value, browserstate.Record{
		SubjectID: initial.SubjectID, Tokens: goauth.TokenPair{AccessToken: testOldAccessToken, RefreshToken: testOldRefreshToken},
	}, time.Hour))
	var refreshes atomic.Int32
	svc := NewService(store, roleServiceStub{}, WithRuntime(&runtimeStub{
		verify: func(context.Context, string, bool) (goauth.AuthContext, error) {
			return goauth.AuthContext{}, goauth.ErrExpiredToken
		},
		refresh: func(context.Context, string) (goauth.TokenPair, error) {
			refreshes.Add(1)
			return goauth.TokenPair{}, goauth.ErrOperationOutcomeUnknown
		},
	}, adminRepositoryStub{}), WithBrowserState(journal))
	app.Get("/unknown", func(c fiber.Ctx) error {
		_, err := svc.GetAdminAuth(c)
		if !errors.Is(err, goauth.ErrOperationOutcomeUnknown) {
			return c.SendStatus(500)
		}
		return c.SendStatus(204)
	})
	response := performSessionRequest(t, app, http.MethodGet, "/unknown", cookie)
	require.Equal(t, 204, response.StatusCode)
	require.NoError(t, response.Body.Close())
	record, found, err := journal.Load(t.Context(), cookie.Value)
	require.NoError(t, err)
	require.True(t, found)
	require.NotEmpty(t, record.Owner)
	claimed, err := journal.Claim(t.Context(), cookie.Value, record.Version, "another-owner")
	require.NoError(t, err)
	require.False(t, claimed)
	require.EqualValues(t, 1, refreshes.Load())
}
