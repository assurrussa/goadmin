//nolint:testpackage // exercises the private canonical logout and browser projection boundaries.
package adminservice

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/testkit"
	"github.com/gofiber/fiber/v3"
	fibersession "github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/internal/auth/browserstate"
	"github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/internal/realtimesession"
	"github.com/assurrussa/goadmin/models"
)

func observeRealtimeClose(t *testing.T, service *Service, key realtimesession.Key) *atomic.Int32 {
	t.Helper()
	admission := service.BeginRealtimeAdmission()
	t.Cleanup(admission.Cancel)
	lease := admission.Bind(key, time.Minute)
	require.NotNil(t, lease)
	t.Cleanup(lease.Release)
	closed := new(atomic.Int32)
	require.True(t, lease.Attach(func() { closed.Add(1) }))
	return closed
}

func TestExplicitLogoutClosesOnlyCanonicalRealtimeSession(t *testing.T) {
	const journalAuthority = "journal"
	for _, authority := range []string{"browser", journalAuthority} {
		for _, outcome := range []string{"success", "already-revoked"} {
			t.Run(authority+"/"+outcome, func(t *testing.T) {
				store := fibersession.NewStore()
				app, cookie := seedAdminBrowserSession(t, store)
				initial := loadBrowserSession(t, store, cookie.Value)
				key := realtimesession.Key{SubjectID: initial.SubjectID, AuthSessionID: initial.AuthSessionID}
				var journal browserstate.Store
				if authority == journalAuthority {
					journal = browserstate.NewMemory()
					key = realtimesession.Key{SubjectID: goauth.NewSubjectID(), AuthSessionID: "journal-session"}
					tokens := goauth.TokenPair{}
					tokens.Session.ID = key.AuthSessionID
					require.NoError(t, journal.Create(t.Context(), cookie.Value, browserstate.Record{
						SubjectID: key.SubjectID, Tokens: tokens,
					}, time.Hour))
				}
				var logoutCalls atomic.Int32
				var matching *atomic.Int32
				service := NewService(store, roleServiceStub{}, WithRuntime(&runtimeStub{
					logout: func(_ context.Context, subject goauth.SubjectID, session string) error {
						logoutCalls.Add(1)
						require.Equal(t, key.SubjectID, subject)
						require.Equal(t, key.AuthSessionID, session)
						require.Zero(t, matching.Load(), "close must follow canonical revocation")
						if outcome == "already-revoked" {
							return goauth.ErrSessionRevoked
						}
						return nil
					},
				}, adminRepositoryStub{}))
				service.states = journal
				staleProjection := new(atomic.Int32)
				if authority == journalAuthority {
					staleProjection = observeRealtimeClose(t, service, realtimesession.Key{
						SubjectID: initial.SubjectID, AuthSessionID: initial.AuthSessionID,
					})
				}
				matching = observeRealtimeClose(t, service, key)
				secondSocket := observeRealtimeClose(t, service, key)
				otherSession := observeRealtimeClose(t, service, realtimesession.Key{
					SubjectID: key.SubjectID, AuthSessionID: "independent-session",
				})
				otherSubject := observeRealtimeClose(t, service, realtimesession.Key{
					SubjectID: goauth.NewSubjectID(), AuthSessionID: key.AuthSessionID,
				})
				otherRuntime := observeRealtimeClose(t, NewService(nil, roleServiceStub{}), key)
				staleAuth := service.BeginRealtimeAdmission()
				t.Cleanup(staleAuth.Cancel)
				pending := service.BeginRealtimeAdmission().Bind(key, time.Minute)
				require.NotNil(t, pending)
				t.Cleanup(pending.Release)
				app.Delete("/logout", func(c fiber.Ctx) error {
					require.NoError(t, service.DelAdminAuth(c))
					return c.SendStatus(http.StatusNoContent)
				})

				for range 2 {
					response := performSessionRequest(t, app, http.MethodDelete, "/logout", cookie)
					require.NoError(t, response.Body.Close())
				}
				require.EqualValues(t, 1, logoutCalls.Load())
				require.EqualValues(t, 1, matching.Load())
				require.EqualValues(t, 1, secondSocket.Load())
				require.Zero(t, otherSession.Load())
				require.Zero(t, otherSubject.Load())
				require.Zero(t, otherRuntime.Load())
				require.Zero(t, staleProjection.Load())
				require.Nil(t, staleAuth.Bind(key, time.Minute))
				require.False(t, pending.Attach(func() { t.Error("revoked pending socket attached") }))
			})
		}
	}
}

func TestRealtimeLogoutCanonicalOutcomeSurvivesBrowserCleanup(t *testing.T) {
	const browserSaveFailed = "browser-save-failed"
	for _, mode := range []string{browserSaveFailed, "runtime-logout-failed"} {
		t.Run(mode, func(t *testing.T) {
			store := fibersession.NewStore()
			storage := &failingSetStorage{Storage: store.Storage}
			store.Storage = storage
			app, cookie := seedAdminBrowserSession(t, store)
			state := loadBrowserSession(t, store, cookie.Value)
			want := errors.New("injected logout failure")
			service := NewService(store, roleServiceStub{}, WithRuntime(&runtimeStub{
				logout: func(context.Context, goauth.SubjectID, string) error {
					if mode == "runtime-logout-failed" {
						return want
					}
					return nil
				},
			}, adminRepositoryStub{}))
			key := realtimesession.Key{SubjectID: state.SubjectID, AuthSessionID: state.AuthSessionID}
			closed := observeRealtimeClose(t, service, key)
			staleAuth := service.BeginRealtimeAdmission()
			t.Cleanup(staleAuth.Cancel)
			if mode == browserSaveFailed {
				storage.failErr = want
			}
			app.Delete("/logout", func(c fiber.Ctx) error {
				require.ErrorIs(t, service.DelAdminAuth(c), want)
				return c.SendStatus(http.StatusNoContent)
			})
			response := performSessionRequest(t, app, http.MethodDelete, "/logout", cookie)
			require.NoError(t, response.Body.Close())
			late := staleAuth.Bind(key, time.Minute)
			if mode == browserSaveFailed {
				require.EqualValues(t, 1, closed.Load())
				require.Nil(t, late, "browser persistence cannot undo canonical revocation")
			} else {
				require.Zero(t, closed.Load())
				require.NotNil(t, late, "failed canonical logout must not add realtime revocation")
				t.Cleanup(late.Release)
			}
		})
	}
}

type observedRealtimeExternalLogout struct {
	ExternalSessionAuthority
	beforeLogout func()
}

func (a observedRealtimeExternalLogout) Logout(ctx context.Context, binding ExternalBinding) error {
	a.beforeLogout()
	return a.ExternalSessionAuthority.Logout(ctx, binding)
}

func TestRealtimeExternalLogoutClosesBeforeProviderFailure(t *testing.T) {
	fixture := newExternalFixture(t)
	cookie := admittedExternal(t, fixture)
	state := loadBrowserSession(t, fixture.service.store, cookie.Value)
	key := realtimesession.Key{SubjectID: state.SubjectID, AuthSessionID: state.AuthSessionID}
	closed := observeRealtimeClose(t, fixture.service, key)
	fixture.authority.err = errors.New("injected provider logout failure")
	fixture.service.externalAuthority = observedRealtimeExternalLogout{
		ExternalSessionAuthority: fixture.authority,
		beforeLogout: func() {
			require.EqualValues(t, 1, closed.Load(), "provider cleanup must follow socket closure")
		},
	}
	response, err := fixture.app.Test(requestWithCookie(http.MethodDelete, "/logout", cookie))
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, response.StatusCode)
	require.NoError(t, response.Body.Close())
	require.Equal(t, 1, fixture.authority.invalidated)
	require.EqualValues(t, 1, closed.Load())
}

func TestRealtimeExternalDetachDoesNotAcquireLogoutPolicy(t *testing.T) {
	fixture := newExternalFixture(t)
	cookie := admittedExternal(t, fixture)
	state := loadBrowserSession(t, fixture.service.store, cookie.Value)
	key := realtimesession.Key{SubjectID: state.SubjectID, AuthSessionID: state.AuthSessionID}
	closed := observeRealtimeClose(t, fixture.service, key)
	staleAuth := fixture.service.BeginRealtimeAdmission()
	t.Cleanup(staleAuth.Cancel)
	fixture.app.Delete("/detach", func(c fiber.Ctx) error {
		require.NoError(t, fixture.service.DetachAdminAuth(c))
		return c.SendStatus(http.StatusNoContent)
	})
	response := performSessionRequest(t, fixture.app, http.MethodDelete, "/detach", cookie)
	require.NoError(t, response.Body.Close())
	require.Equal(t, 1, fixture.authority.detached)
	require.Zero(t, fixture.authority.invalidated)
	require.Zero(t, closed.Load())
	late := staleAuth.Bind(key, time.Minute)
	require.NotNil(t, late)
	t.Cleanup(late.Release)
}

func TestRealtimeProjectionUsesIntrospectedCanonicalSession(t *testing.T) {
	for _, refresh := range []bool{false, true} {
		name := "current-access"
		if refresh {
			name = "refreshed-access"
		}
		t.Run(name, func(t *testing.T) {
			store := fibersession.NewStore()
			app, cookie := seedAdminBrowserSession(t, store)
			state := loadBrowserSession(t, store, cookie.Value)
			var refreshes atomic.Int32
			service := NewService(store, roleServiceStub{}, WithRuntime(&runtimeStub{
				verify: func(_ context.Context, token string, introspect bool) (goauth.AuthContext, error) {
					require.True(t, introspect)
					if refresh && token != testNewAccessToken {
						return goauth.AuthContext{}, goauth.ErrExpiredToken
					}
					return goauth.AuthContext{
						SubjectID: state.SubjectID, Realm: goauth.RealmAdmin, SessionID: "verified-session",
					}, nil
				},
				refresh: func(context.Context, string) (goauth.TokenPair, error) {
					refreshes.Add(1)
					tokens := goauth.TokenPair{AccessToken: testNewAccessToken, RefreshToken: testNewRefreshToken}
					tokens.Session.ID = "unverified-refresh-envelope"
					return tokens, nil
				},
				getAccount: func(context.Context, goauth.SubjectID) (goauth.Account, error) {
					return goauth.Account{}, nil
				},
			}, adminRepositoryStub{admin: models.Admin{ID: 1, SubjectID: state.SubjectID, UUID: identity.NewUserID()}}))
			app.Get("/auth", func(c fiber.Ctx) error {
				actor, err := service.GetAdminAuth(c)
				require.NoError(t, err)
				require.NotNil(t, actor)
				require.Equal(t, "verified-session", actor.AuthSessionID)
				require.Equal(t, state.SubjectID, actor.SubjectID)
				require.Equal(t, cookie.Value, actor.SessionID)
				return c.SendStatus(http.StatusNoContent)
			})
			response := performSessionRequest(t, app, http.MethodGet, "/auth", cookie)
			require.NoError(t, response.Body.Close())
			if refresh {
				require.EqualValues(t, 1, refreshes.Load())
			} else {
				require.Zero(t, refreshes.Load())
			}
		})
	}
}

func TestRealtimeCanonicalSessionSurvivesRefreshRotationAndNewLogin(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	fixture, err := testkit.NewRuntime(func(cfg *goauth.Config) {
		cfg.AccessTTL = time.Second
		cfg.Now = func() time.Time { return now }
	})
	require.NoError(t, err)
	account, err := fixture.Runtime.ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
		Email: "realtime-logout@example.test", Password: testAdminPassword,
	})
	require.NoError(t, err)
	admin := models.Admin{ID: 91, SubjectID: account.Subject.ID, UUID: identity.NewUserID()}
	store := fibersession.NewStore()
	journal := browserstate.NewMemory()
	service := NewService(store, roleServiceStub{},
		WithRuntime(fixture.Runtime, adminRepositoryStub{admin: admin}), WithBrowserState(journal))
	app := fiber.New()
	app.Post("/login", func(c fiber.Ctx) error {
		_, _, loginErr := service.LoginAdmin(c, account.PrimaryEmail.DisplayValue, testAdminPassword, false)
		require.NoError(t, loginErr)
		return c.SendStatus(http.StatusNoContent)
	})
	var actor models.SessionAdmin
	app.Get("/auth", func(c fiber.Ctx) error {
		authenticated, authErr := service.GetAdminAuth(c)
		require.NoError(t, authErr)
		require.NotNil(t, authenticated)
		actor = *authenticated
		return c.SendStatus(http.StatusNoContent)
	})
	app.Post("/rotate", func(c fiber.Ctx) error {
		id, rotateErr := service.RotateAdminAuth(c, admin)
		require.NoError(t, rotateErr)
		service.writeSessionCookie(c, id, false)
		return c.SendStatus(http.StatusNoContent)
	})
	app.Delete("/logout", func(c fiber.Ctx) error {
		require.NoError(t, service.DelAdminAuth(c))
		return c.SendStatus(http.StatusNoContent)
	})
	request := func(method, path string, cookie *http.Cookie) http.Header {
		response := performSessionRequest(t, app, method, path, cookie)
		require.NoError(t, response.Body.Close())
		return response.Header
	}
	requestCookie := func(method, path string, cookie *http.Cookie) *http.Cookie {
		// Only closed-response metadata crosses the request helper boundary.
		return requireSessionCookie(t, &http.Response{Header: request(method, path, cookie)})
	}
	cookie := requestCookie(http.MethodPost, "/login", nil)
	request(http.MethodGet, "/auth", cookie)
	initial := actor
	require.NotEmpty(t, initial.AuthSessionID)
	require.NotEqual(t, initial.SessionID, initial.AuthSessionID)
	key := realtimesession.Key{SubjectID: initial.SubjectID, AuthSessionID: initial.AuthSessionID}
	closedBeforeRotation := observeRealtimeClose(t, service, key)
	before, found, err := journal.Load(t.Context(), cookie.Value)
	require.NoError(t, err)
	require.True(t, found)

	now = now.Add(2 * time.Second)
	request(http.MethodGet, "/auth", cookie)
	require.Equal(t, initial.AuthSessionID, actor.AuthSessionID)
	after, found, err := journal.Load(t.Context(), cookie.Value)
	require.NoError(t, err)
	require.True(t, found)
	require.NotEqual(t, before.Tokens.AccessToken, after.Tokens.AccessToken)
	rotatedCookie := requestCookie(http.MethodPost, "/rotate", cookie)
	require.NotEqual(t, cookie.Value, rotatedCookie.Value)
	request(http.MethodGet, "/auth", rotatedCookie)
	require.Equal(t, initial.AuthSessionID, actor.AuthSessionID)
	require.Equal(t, rotatedCookie.Value, actor.SessionID)
	closedAfterRotation := observeRealtimeClose(t, service, key)

	otherCookie := requestCookie(http.MethodPost, "/login", nil)
	request(http.MethodGet, "/auth", otherCookie)
	require.Equal(t, initial.SubjectID, actor.SubjectID)
	require.Equal(t, initial.UUID, actor.UUID)
	require.NotEqual(t, initial.AuthSessionID, actor.AuthSessionID)
	independentKey := realtimesession.Key{SubjectID: actor.SubjectID, AuthSessionID: actor.AuthSessionID}
	independentClosed := observeRealtimeClose(t, service, independentKey)
	request(http.MethodDelete, "/logout", rotatedCookie)
	require.EqualValues(t, 1, closedBeforeRotation.Load())
	require.EqualValues(t, 1, closedAfterRotation.Load())
	require.Zero(t, independentClosed.Load())

	newCookie := requestCookie(http.MethodPost, "/login", nil)
	request(http.MethodGet, "/auth", newCookie)
	require.NotEqual(t, initial.AuthSessionID, actor.AuthSessionID)
	newClosed := observeRealtimeClose(t, service, realtimesession.Key{
		SubjectID: actor.SubjectID, AuthSessionID: actor.AuthSessionID,
	})
	request(http.MethodDelete, "/logout", rotatedCookie)
	require.EqualValues(t, 1, closedBeforeRotation.Load())
	require.EqualValues(t, 1, closedAfterRotation.Load())
	require.Zero(t, independentClosed.Load())
	require.Zero(t, newClosed.Load())
	request(http.MethodGet, "/auth", otherCookie)
	request(http.MethodGet, "/auth", newCookie)
	request(http.MethodDelete, "/logout", newCookie)
	require.EqualValues(t, 1, newClosed.Load())
	require.Zero(t, independentClosed.Load())
}
