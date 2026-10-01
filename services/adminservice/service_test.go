//nolint:testpackage // exercises the private browser-session persistence contract.
package adminservice

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/testkit"
	"github.com/gofiber/fiber/v3"
	fibersession "github.com/gofiber/fiber/v3/middleware/session"
	redisv9 "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/assurrussa/goadmin/config"
	sessioncore "github.com/assurrussa/goadmin/infrastructure/core/session"
	"github.com/assurrussa/goadmin/infrastructure/core/session/sessionredis"
	redismocks "github.com/assurrussa/goadmin/infrastructure/redis/testsupport"
	internalauth "github.com/assurrussa/goadmin/internal/auth"
	identity "github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
)

const (
	testAdminPassword     = "Admin-Unique-Passphrase-123" //nolint:gosec // isolated test credential
	testSessionCookieName = "session_id"
	testCanonicalSession  = "canonical-session"
	testOldAccessToken    = "old-access"
	testNewAccessToken    = "new-access"
	testOldRefreshToken   = "old-refresh"
	testNewRefreshToken   = "new-refresh"
)

func TestAdminCookiePolicyAcrossLoginRefreshAndOtherSessionWrites(t *testing.T) {
	for _, persistent := range []bool{false, true} {
		name := "browser session"
		if persistent {
			name = "remember me"
		}
		t.Run(name, func(t *testing.T) {
			now := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
			fixture, err := testkit.NewRuntime(func(cfg *goauth.Config) {
				cfg.AccessTTL = time.Second
				cfg.Now = func() time.Time { return now }
			})
			require.NoError(t, err)
			account, err := fixture.Runtime.ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
				Email: "cookie-policy@example.test", Password: testAdminPassword,
			})
			require.NoError(t, err)
			admin := models.Admin{ID: 91, SubjectID: account.Subject.ID, UUID: identity.NewUserID()}
			redisClient := newSessionRedisMock(t)
			_, store := sessionredis.CreateAdminSessionStore(redisClient, config.Config{
				SessionName: testSessionCookieName, SessionInactiveTTL: 30 * time.Minute,
				SessionTTL: 24 * time.Hour,
			})
			service := NewService(store, roleServiceStub{}, WithRuntime(fixture.Runtime, adminRepositoryStub{admin: admin}))
			app := fiber.New()
			app.Post("/login", func(c fiber.Ctx) error {
				_, _, loginErr := service.LoginAdmin(c, account.PrimaryEmail.DisplayValue, testAdminPassword, persistent)
				require.NoError(t, loginErr)
				return c.SendStatus(http.StatusNoContent)
			})
			app.Get("/auth", func(c fiber.Ctx) error {
				_, authErr := service.GetAdminAuth(c)
				require.NoError(t, authErr)
				return c.SendStatus(http.StatusNoContent)
			})
			app.Get("/flash", func(c fiber.Ctx) error {
				sess, sessionErr := store.Get(c)
				require.NoError(t, sessionErr)
				defer sess.Release()
				sess.Set("flash", "message")
				require.NoError(t, sess.Save())
				return c.SendStatus(http.StatusNoContent)
			})

			loginResponse := performSessionRequest(t, app, http.MethodPost, "/login", nil)
			sessionCookie := requireSessionCookie(t, loginResponse)
			policyCookie := requireNamedCookie(t, loginResponse, testSessionCookieName+sessionPersistenceCookieSuffix)
			if persistent {
				require.Equal(t, "1", policyCookie.Value)
			} else {
				require.Equal(t, "0", policyCookie.Value)
			}
			assertCookiePolicy(t, sessionCookie, persistent)
			require.NoError(t, loginResponse.Body.Close())

			now = now.Add(2 * time.Second)
			for _, path := range []string{"/auth", "/flash"} {
				request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
				request.AddCookie(sessionCookie)
				request.AddCookie(policyCookie)
				response, requestErr := app.Test(request)
				require.NoError(t, requestErr)
				require.Equal(t, http.StatusNoContent, response.StatusCode)
				updated := requireSessionCookie(t, response)
				require.Equal(t, sessionCookie.Value, updated.Value)
				assertCookiePolicy(t, updated, persistent)
				require.NoError(t, response.Body.Close())
			}
		})
	}
}

func assertCookiePolicy(t *testing.T, cookie *http.Cookie, persistent bool) {
	t.Helper()
	if persistent {
		require.Greater(t, cookie.MaxAge, int((23 * time.Hour).Seconds()))
		return
	}
	require.Zero(t, cookie.MaxAge)
	require.True(t, cookie.Expires.IsZero())
}

func requireNamedCookie(t *testing.T, response *http.Response, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("cookie %q was not returned", name)
	return nil
}

func newSessionRedisMock(t *testing.T) *redismocks.MockClientContract {
	t.Helper()
	client := redismocks.NewMockClientContract(gomock.NewController(t))
	values := make(map[string][]byte)
	client.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, key string) *redisv9.StringCmd {
		value, ok := values[key]
		if !ok {
			return redisv9.NewStringResult("", redisv9.Nil)
		}
		return redisv9.NewStringResult(string(value), nil)
	}).AnyTimes()
	client.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, key string, value any, _ time.Duration) *redisv9.StatusCmd {
			switch value := value.(type) {
			case []byte:
				values[key] = append([]byte(nil), value...)
			default:
				return redisv9.NewStatusResult("", errors.New("unexpected Redis session value type"))
			}
			return redisv9.NewStatusResult("OK", nil)
		},
	).AnyTimes()
	client.EXPECT().Del(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, key string) *redisv9.IntCmd {
		delete(values, key)
		return redisv9.NewIntResult(1, nil)
	}).AnyTimes()
	return client
}

func TestAdminSessionUsesAdminRealmRefreshesAndRevokes(t *testing.T) {
	now := time.Date(2026, 8, 18, 5, 0, 0, 0, time.UTC)
	var gatedRealm goauth.Realm
	fixture, err := testkit.NewRuntime(func(config *goauth.Config) {
		config.AccessTTL = time.Second
		config.Now = func() time.Time { return now }
		config.MembershipGate = goauth.MembershipGateFunc(
			func(_ context.Context, realm goauth.Realm, _ goauth.Account) error {
				gatedRealm = realm
				return nil
			},
		)
	})
	require.NoError(t, err)
	account, err := fixture.Runtime.ProvisionTrustedLocalAccount(context.Background(), goauth.RegisterRequest{
		Email: "admin.runtime@example.test", Password: testAdminPassword,
		Profile: goauth.BasicProfile{Username: "admin-runtime", DisplayName: "Admin Runtime"},
	})
	require.NoError(t, err)

	admin := models.Admin{
		ID: 41, SubjectID: account.Subject.ID, UUID: identity.NewUserID(),
		Username: "admin-runtime", Email: account.PrimaryEmail.DisplayValue,
	}
	store := fibersession.NewStore()
	service := NewService(store, roleServiceStub{}, WithRuntime(fixture.Runtime, adminRepositoryStub{admin: admin}))
	app := fiber.New()
	app.Post("/login", func(c fiber.Ctx) error {
		_, loggedIn, loginErr := service.LoginAdmin(
			c,
			account.PrimaryEmail.DisplayValue,
			testAdminPassword,
			false,
		)
		require.NoError(t, loginErr)
		require.Equal(t, account.Subject.ID, loggedIn.Subject.ID)

		return c.SendStatus(http.StatusNoContent)
	})
	app.Get("/auth", func(c fiber.Ctx) error {
		authenticated, authErr := service.GetAdminAuth(c)
		require.NoError(t, authErr)
		require.NotNil(t, authenticated)
		require.Equal(t, account.Subject.ID, authenticated.SubjectID)

		return c.SendStatus(http.StatusNoContent)
	})
	app.Delete("/logout", func(c fiber.Ctx) error {
		require.NoError(t, service.DelAdminAuth(c))

		return c.SendStatus(http.StatusNoContent)
	})

	loginResponse := performSessionRequest(t, app, http.MethodPost, "/login", nil)
	sessionCookie := requireSessionCookie(t, loginResponse)
	require.NoError(t, loginResponse.Body.Close())
	require.Equal(t, goauth.RealmAdmin, gatedRealm)
	initial := loadBrowserSession(t, store, sessionCookie.Value)
	initialContext, err := fixture.Runtime.AuthenticateSession(context.Background(), initial.AccessToken)
	require.NoError(t, err)
	require.Equal(t, goauth.RealmAdmin, initialContext.Realm)

	now = now.Add(2 * time.Second)
	authResponse := performSessionRequest(t, app, http.MethodGet, "/auth", sessionCookie)
	require.NoError(t, authResponse.Body.Close())
	rotated := loadBrowserSession(t, store, sessionCookie.Value)
	require.NotEqual(t, initial.AccessToken, rotated.AccessToken)
	require.NotEqual(t, initial.RefreshToken, rotated.RefreshToken)
	rotatedContext, err := fixture.Runtime.AuthenticateSession(context.Background(), rotated.AccessToken)
	require.NoError(t, err)
	require.Equal(t, goauth.RealmAdmin, rotatedContext.Realm)

	logoutResponse := performSessionRequest(t, app, http.MethodDelete, "/logout", sessionCookie)
	require.NoError(t, logoutResponse.Body.Close())
	_, err = fixture.Runtime.AuthenticateSession(context.Background(), rotated.AccessToken)
	require.ErrorIs(t, err, goauth.ErrSessionRevoked)
}

func TestAdminSessionRejectsUnverifiedEmail(t *testing.T) {
	fixture, err := testkit.NewRuntime()
	require.NoError(t, err)
	registered, err := fixture.Runtime.Register(context.Background(), goauth.RegisterRequest{
		Email: "unverified.admin@example.test", Password: testAdminPassword,
	})
	require.NoError(t, err)

	admin := models.Admin{
		ID: 43, SubjectID: registered.Account.Subject.ID, UUID: identity.NewUserID(),
		Email: registered.Account.PrimaryEmail.DisplayValue,
	}
	service := NewService(
		fibersession.NewStore(),
		roleServiceStub{},
		WithRuntime(fixture.Runtime, adminRepositoryStub{admin: admin}),
	)
	app := fiber.New()
	app.Post("/login", func(c fiber.Ctx) error {
		_, _, loginErr := service.LoginAdmin(c, admin.Email, testAdminPassword, false)
		require.ErrorIs(t, loginErr, goauth.ErrEmailVerificationRequired)

		return c.SendStatus(http.StatusNoContent)
	})

	response := performSessionRequest(t, app, http.MethodPost, "/login", nil)
	require.NoError(t, response.Body.Close())
}

func TestAdminSessionRejectsMissingMembership(t *testing.T) {
	var gatedRealm goauth.Realm
	fixture, err := testkit.NewRuntime(func(config *goauth.Config) {
		config.MembershipGate = goauth.MembershipGateFunc(
			func(_ context.Context, realm goauth.Realm, _ goauth.Account) error {
				gatedRealm = realm

				return goauth.ErrMembershipDenied
			},
		)
	})
	require.NoError(t, err)
	account, err := fixture.Runtime.ProvisionTrustedLocalAccount(context.Background(), goauth.RegisterRequest{
		Email: "non-member.admin@example.test", Password: testAdminPassword,
	})
	require.NoError(t, err)

	admin := models.Admin{
		ID: 44, SubjectID: account.Subject.ID, UUID: identity.NewUserID(),
		Email: account.PrimaryEmail.DisplayValue,
	}
	service := NewService(
		fibersession.NewStore(),
		roleServiceStub{},
		WithRuntime(fixture.Runtime, adminRepositoryStub{admin: admin}),
	)
	app := fiber.New()
	app.Post("/login", func(c fiber.Ctx) error {
		_, _, loginErr := service.LoginAdmin(c, admin.Email, testAdminPassword, false)
		require.ErrorIs(t, loginErr, goauth.ErrMembershipDenied)

		return c.SendStatus(http.StatusNoContent)
	})

	response := performSessionRequest(t, app, http.MethodPost, "/login", nil)
	require.NoError(t, response.Body.Close())
	require.Equal(t, goauth.RealmAdmin, gatedRealm)
}

func TestAdminSessionRejectsUserRealmToken(t *testing.T) {
	fixture, err := testkit.NewRuntime()
	require.NoError(t, err)
	account, err := fixture.Runtime.ProvisionTrustedLocalAccount(context.Background(), goauth.RegisterRequest{
		Email: "user.realm@example.test", Password: "User-Unique-Passphrase-123",
	})
	require.NoError(t, err)
	login, err := fixture.Runtime.Login(context.Background(), goauth.LoginRequest{
		Credential: goauth.Credential{
			Identifier: goauth.IdentifierInput{
				Scheme: goauth.IdentifierSchemeEmail,
				Value:  account.PrimaryEmail.DisplayValue,
			},
			Password: "User-Unique-Passphrase-123",
		},
		Realm: goauth.RealmUser,
	})
	require.NoError(t, err)

	admin := models.Admin{ID: 42, SubjectID: account.Subject.ID, UUID: identity.NewUserID()}
	store := fibersession.NewStore()
	service := NewService(store, roleServiceStub{}, WithRuntime(fixture.Runtime, adminRepositoryStub{admin: admin}))
	app := fiber.New()
	app.Get("/seed", func(c fiber.Ctx) error {
		sess, sessionErr := store.Get(c)
		require.NoError(t, sessionErr)
		defer sess.Release()
		require.NoError(t, sess.Regenerate())
		sess.Set(sessioncore.AuthAdminKey.String(), &browserSession{
			AccessToken: login.Tokens.AccessToken, RefreshToken: login.Tokens.RefreshToken,
			AuthSessionID: login.Tokens.Session.ID, SubjectID: account.Subject.ID,
		})
		require.NoError(t, sess.Save())
		c.Cookie(&fiber.Cookie{Name: testSessionCookieName, Value: sess.ID()})

		return c.SendStatus(http.StatusNoContent)
	})
	app.Get("/auth", func(c fiber.Ctx) error {
		authenticated, authErr := service.GetAdminAuth(c)
		require.Nil(t, authenticated)
		require.ErrorIs(t, authErr, goauth.ErrInvalidToken)

		return c.SendStatus(http.StatusNoContent)
	})

	seedResponse := performSessionRequest(t, app, http.MethodGet, "/seed", nil)
	sessionCookie := requireSessionCookie(t, seedResponse)
	require.NoError(t, seedResponse.Body.Close())
	authResponse := performSessionRequest(t, app, http.MethodGet, "/auth", sessionCookie)
	require.NoError(t, authResponse.Body.Close())
}

func TestAdminAuthDoesNotRefreshOnUnexpectedVerificationFailure(t *testing.T) {
	verifyErr := errors.New("session store unavailable")
	store := fibersession.NewStore()
	service := NewService(store, roleServiceStub{}, WithRuntime(&runtimeStub{
		verify: func(context.Context, string, bool) (goauth.AuthContext, error) {
			return goauth.AuthContext{}, verifyErr
		},
	}, adminRepositoryStub{}))
	app, cookie := seedAdminBrowserSession(t, store)
	app.Get("/auth", func(c fiber.Ctx) error {
		_, err := service.GetAdminAuth(c)
		require.ErrorIs(t, err, verifyErr)
		return c.SendStatus(http.StatusNoContent)
	})
	response := performSessionRequest(t, app, http.MethodGet, "/auth", cookie)
	require.NoError(t, response.Body.Close())
	require.Equal(t, "access", loadBrowserSession(t, store, cookie.Value).AccessToken)
}

func TestAdminAuthClearsBrowserSessionForTerminalVerificationErrors(t *testing.T) {
	for _, verifyErr := range []error{
		goauth.ErrSessionRevoked,
		goauth.ErrSecurityVersionMismatch,
		goauth.ErrAccountUnavailable,
		goauth.ErrAccountNotFound,
	} {
		t.Run(verifyErr.Error(), func(t *testing.T) {
			store := fibersession.NewStore()
			app, cookie := seedAdminBrowserSession(t, store)
			service := NewService(store, roleServiceStub{}, WithRuntime(&runtimeStub{
				verify: func(context.Context, string, bool) (goauth.AuthContext, error) {
					return goauth.AuthContext{}, verifyErr
				},
			}, adminRepositoryStub{}))
			app.Get("/auth", func(c fiber.Ctx) error {
				admin, err := service.GetAdminAuth(c)
				require.NoError(t, err)
				require.Nil(t, admin)
				return c.SendStatus(http.StatusNoContent)
			})
			response := performSessionRequest(t, app, http.MethodGet, "/auth", cookie)
			require.Negative(t, requireSessionCookie(t, response).MaxAge)
			require.NoError(t, response.Body.Close())
			sess, err := store.GetByID(t.Context(), cookie.Value)
			require.NoError(t, err)
			t.Cleanup(sess.Release)
			require.Nil(t, sess.Get(sessioncore.AuthAdminKey.String()))
		})
	}
}

func TestAdminAuthPreservesMembershipStoreError(t *testing.T) {
	want := errors.New("membership store unavailable")
	store := fibersession.NewStore()
	app, cookie := seedAdminBrowserSession(t, store)
	state := loadBrowserSession(t, store, cookie.Value)
	service := NewService(store, roleServiceStub{}, WithRuntime(&runtimeStub{
		verify: func(context.Context, string, bool) (goauth.AuthContext, error) {
			return goauth.AuthContext{Realm: goauth.RealmAdmin, SubjectID: state.SubjectID}, nil
		},
		getAccount: func(context.Context, goauth.SubjectID) (goauth.Account, error) {
			return goauth.Account{}, nil
		},
	}, failingAdminRepository{err: want}))
	app.Get("/auth", func(c fiber.Ctx) error {
		_, err := service.GetAdminAuth(c)
		require.ErrorIs(t, err, want)
		return c.SendStatus(http.StatusNoContent)
	})
	response := performSessionRequest(t, app, http.MethodGet, "/auth", cookie)
	require.NoError(t, response.Body.Close())
}

func TestAdminAuthClearsBrowserSessionWhenMembershipIsRemoved(t *testing.T) {
	store := fibersession.NewStore()
	app, cookie := seedAdminBrowserSession(t, store)
	state := loadBrowserSession(t, store, cookie.Value)
	service := NewService(store, roleServiceStub{}, WithRuntime(&runtimeStub{
		verify: func(context.Context, string, bool) (goauth.AuthContext, error) {
			return goauth.AuthContext{Realm: goauth.RealmAdmin, SubjectID: state.SubjectID}, nil
		},
		getAccount: func(context.Context, goauth.SubjectID) (goauth.Account, error) {
			return goauth.Account{}, nil
		},
	}, adminRepositoryStub{admin: models.Admin{SubjectID: state.SubjectID}}))
	app.Get("/auth", func(c fiber.Ctx) error {
		admin, err := service.GetAdminAuth(c)
		require.NoError(t, err)
		require.Nil(t, admin)
		return c.SendStatus(http.StatusNoContent)
	})
	response := performSessionRequest(t, app, http.MethodGet, "/auth", cookie)
	require.Negative(t, requireSessionCookie(t, response).MaxAge)
	require.NoError(t, response.Body.Close())
	sess, err := store.GetByID(t.Context(), cookie.Value)
	require.NoError(t, err)
	defer sess.Release()
	require.Nil(t, sess.Get(sessioncore.AuthAdminKey.String()))
}

func TestAdminAuthRefreshFailurePreservesSessionOnInfrastructureError(t *testing.T) {
	want := errors.New("refresh store unavailable")
	store := fibersession.NewStore()
	service := NewService(store, roleServiceStub{}, WithRuntime(&runtimeStub{
		verify: func(context.Context, string, bool) (goauth.AuthContext, error) {
			return goauth.AuthContext{}, goauth.ErrExpiredToken
		},
		refresh: func(context.Context, string) (goauth.TokenPair, error) {
			return goauth.TokenPair{}, want
		},
	}, adminRepositoryStub{}))
	app, cookie := seedAdminBrowserSession(t, store)
	app.Get("/auth", func(c fiber.Ctx) error {
		_, err := service.GetAdminAuth(c)
		require.ErrorIs(t, err, want)
		return c.SendStatus(http.StatusNoContent)
	})
	response := performSessionRequest(t, app, http.MethodGet, "/auth", cookie)
	require.NoError(t, response.Body.Close())
	require.Equal(t, "refresh", loadBrowserSession(t, store, cookie.Value).RefreshToken)
}

func TestAdminAuthInvalidRefreshClearsLocalSession(t *testing.T) {
	store := fibersession.NewStore()
	service := NewService(store, roleServiceStub{}, WithRuntime(&runtimeStub{
		verify: func(context.Context, string, bool) (goauth.AuthContext, error) {
			return goauth.AuthContext{}, goauth.ErrExpiredToken
		},
		refresh: func(context.Context, string) (goauth.TokenPair, error) {
			return goauth.TokenPair{}, goauth.ErrSessionRevoked
		},
	}, adminRepositoryStub{}))
	app, cookie := seedAdminBrowserSession(t, store)
	app.Get("/auth", func(c fiber.Ctx) error {
		admin, err := service.GetAdminAuth(c)
		require.NoError(t, err)
		require.Nil(t, admin)
		return c.SendStatus(http.StatusNoContent)
	})
	response := performSessionRequest(t, app, http.MethodGet, "/auth", cookie)
	require.NoError(t, response.Body.Close())
	sess, err := store.GetByID(t.Context(), cookie.Value)
	require.NoError(t, err)
	defer sess.Release()
	require.Nil(t, sess.Get(sessioncore.AuthAdminKey.String()))
}

func TestAdminAuthInvalidRefreshReportsLocalSaveFailure(t *testing.T) {
	want := errors.New("browser session store unavailable")
	store := fibersession.NewStore()
	storage := &failingSetStorage{Storage: store.Storage}
	store.Storage = storage
	service := NewService(store, roleServiceStub{}, WithRuntime(&runtimeStub{
		verify: func(context.Context, string, bool) (goauth.AuthContext, error) {
			return goauth.AuthContext{}, goauth.ErrExpiredToken
		},
		refresh: func(context.Context, string) (goauth.TokenPair, error) {
			return goauth.TokenPair{}, goauth.ErrSessionRevoked
		},
	}, adminRepositoryStub{}))
	app, cookie := seedAdminBrowserSession(t, store)
	storage.failErr = want
	app.Get("/auth", func(c fiber.Ctx) error {
		_, err := service.GetAdminAuth(c)
		require.ErrorIs(t, err, want)
		return c.SendStatus(http.StatusNoContent)
	})
	response := performSessionRequest(t, app, http.MethodGet, "/auth", cookie)
	require.NoError(t, response.Body.Close())
}

func TestAdminLogoutClearsBrowserSessionWhenRuntimeFails(t *testing.T) {
	want := errors.New("revocation store unavailable")
	store := fibersession.NewStore()
	service := NewService(store, roleServiceStub{}, WithRuntime(&runtimeStub{
		logout: func(context.Context, goauth.SubjectID, string) error { return want },
	}, adminRepositoryStub{}))
	app, cookie := seedAdminBrowserSession(t, store)
	app.Delete("/logout", func(c fiber.Ctx) error {
		require.ErrorIs(t, service.DelAdminAuth(c), want)
		return c.SendStatus(http.StatusNoContent)
	})
	response := performSessionRequest(t, app, http.MethodDelete, "/logout", cookie)
	require.NoError(t, response.Body.Close())
	sess, err := store.GetByID(t.Context(), cookie.Value)
	require.NoError(t, err)
	defer sess.Release()
	require.Nil(t, sess.Get(sessioncore.AuthAdminKey.String()))
	var policyDeleted bool
	for _, responseCookie := range response.Cookies() {
		if responseCookie.Name == testSessionCookieName+sessionPersistenceCookieSuffix {
			policyDeleted = responseCookie.MaxAge < 0
		}
	}
	require.True(t, policyDeleted)
}

func TestProvisionTrustedAdminPreservesCredentialStoreError(t *testing.T) {
	want := errors.New("credential store unavailable")
	service := NewService(nil, roleServiceStub{}, WithRuntime(&runtimeStub{
		provision: func(context.Context, goauth.RegisterRequest) (goauth.Account, error) {
			return goauth.Account{}, goauth.ErrIdentifierAlreadyExists
		},
		credential: func(context.Context, goauth.Credential) (goauth.Account, error) {
			return goauth.Account{}, want
		},
	}, adminRepositoryStub{}))
	_, err := service.ProvisionTrustedAdmin(t.Context(), goauth.RegisterRequest{Email: "admin@example.test"})
	require.ErrorIs(t, err, want)
}

func TestDeleteAllExceptRevokesAllSessions(t *testing.T) {
	subjectID := goauth.NewSubjectID()
	service := NewService(nil, roleServiceStub{}, WithRuntime(&runtimeStub{
		logoutAll: func(_ context.Context, got goauth.SubjectID) (int64, error) {
			require.Equal(t, subjectID, got)
			return 3, nil
		},
	}, adminRepositoryStub{}))
	deleted, err := service.DeleteAllExcept(t.Context(), subjectID, "ignored-session")
	require.NoError(t, err)
	require.EqualValues(t, 3, deleted)
}

type runtimeStub struct {
	runtime
	verify     func(context.Context, string, bool) (goauth.AuthContext, error)
	refresh    func(context.Context, string) (goauth.TokenPair, error)
	getAccount func(context.Context, goauth.SubjectID) (goauth.Account, error)
	logout     func(context.Context, goauth.SubjectID, string) error
	logoutAll  func(context.Context, goauth.SubjectID) (int64, error)
	provision  func(context.Context, goauth.RegisterRequest) (goauth.Account, error)
	credential func(context.Context, goauth.Credential) (goauth.Account, error)
}

type failingSetStorage struct {
	fiber.Storage
	failErr error
}

func (s *failingSetStorage) SetWithContext(ctx context.Context, key string, value []byte, exp time.Duration) error {
	if s.failErr != nil {
		return s.failErr
	}
	return s.Storage.SetWithContext(ctx, key, value, exp)
}

func (s *runtimeStub) VerifyAccessToken(ctx context.Context, token string, introspect bool) (goauth.AuthContext, error) {
	return s.verify(ctx, token, introspect)
}

func (s *runtimeStub) Refresh(ctx context.Context, token string) (goauth.TokenPair, error) {
	return s.refresh(ctx, token)
}

func (s *runtimeStub) GetAccount(ctx context.Context, subjectID goauth.SubjectID) (goauth.Account, error) {
	return s.getAccount(ctx, subjectID)
}

func (s *runtimeStub) Logout(ctx context.Context, subjectID goauth.SubjectID, sessionID string) error {
	return s.logout(ctx, subjectID, sessionID)
}

func (s *runtimeStub) LogoutAll(ctx context.Context, subjectID goauth.SubjectID) (int64, error) {
	return s.logoutAll(ctx, subjectID)
}

func (s *runtimeStub) ProvisionTrustedLocalAccount(ctx context.Context, request goauth.RegisterRequest) (goauth.Account, error) {
	return s.provision(ctx, request)
}

func (s *runtimeStub) VerifyCredential(ctx context.Context, credential goauth.Credential) (goauth.Account, error) {
	return s.credential(ctx, credential)
}

type failingAdminRepository struct{ err error }

func (r failingAdminRepository) GetBySubjectID(context.Context, goauth.SubjectID) (models.Admin, error) {
	return models.Admin{}, r.err
}

func seedAdminBrowserSession(t *testing.T, store *fibersession.Store) (*fiber.App, *http.Cookie) {
	t.Helper()
	app := fiber.New()
	app.Get("/seed", func(c fiber.Ctx) error {
		sess, err := store.Get(c)
		require.NoError(t, err)
		defer sess.Release()
		require.NoError(t, sess.Regenerate())
		sess.Set(sessioncore.AuthAdminKey.String(), &browserSession{
			AccessToken: "access", RefreshToken: "refresh", AuthSessionID: testCanonicalSession,
			SubjectID: goauth.NewSubjectID(),
		})
		require.NoError(t, sess.Save())
		c.Cookie(&fiber.Cookie{Name: testSessionCookieName, Value: sess.ID()})
		return c.SendStatus(http.StatusNoContent)
	})
	response := performSessionRequest(t, app, http.MethodGet, "/seed", nil)
	cookie := requireSessionCookie(t, response)
	require.NoError(t, response.Body.Close())
	return app, cookie
}

type roleServiceStub struct{}

func (roleServiceStub) GetRolesAdmin(context.Context, int64) ([]internalauth.Role, error) {
	return nil, nil
}

func (roleServiceStub) GetPermissions(
	context.Context,
	[]internalauth.Role,
) (map[int64]map[int64]internalauth.Permission, error) {
	return map[int64]map[int64]internalauth.Permission{}, nil
}

type adminRepositoryStub struct{ admin models.Admin }

func (s adminRepositoryStub) GetBySubjectID(
	_ context.Context,
	subjectID goauth.SubjectID,
) (models.Admin, error) {
	if s.admin.SubjectID != subjectID {
		return models.Admin{}, errors.New("admin membership not found")
	}

	return s.admin, nil
}

func performSessionRequest(
	t *testing.T,
	app *fiber.App,
	method string,
	path string,
	cookie *http.Cookie,
) *http.Response {
	t.Helper()
	request := httptest.NewRequestWithContext(context.Background(), method, path, nil)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response, err := app.Test(request)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, response.StatusCode)

	return response
}

func requireSessionCookie(t *testing.T, response *http.Response) *http.Cookie {
	t.Helper()
	var found *http.Cookie
	for _, cookie := range response.Cookies() {
		if cookie.Name == testSessionCookieName {
			require.Nil(t, found, "duplicate session cookie")
			found = cookie
		}
	}
	require.NotNil(t, found, "session cookie was not returned")
	return found
}

func loadBrowserSession(t *testing.T, store *fibersession.Store, sessionID string) browserSession {
	t.Helper()
	sess, err := store.GetByID(context.Background(), sessionID)
	require.NoError(t, err)
	defer sess.Release()
	state, ok := sess.Get(sessioncore.AuthAdminKey.String()).(*browserSession)
	require.True(t, ok)
	require.NotNil(t, state)

	return *state
}
