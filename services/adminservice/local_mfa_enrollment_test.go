//nolint:testpackage // Exercises private pending ownership and exact browser publication.
package adminservice

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	fibersession "github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/require"

	sessioncore "github.com/assurrussa/goadmin/infrastructure/core/session"
	"github.com/assurrussa/goadmin/internal/auth/browserstate"
	"github.com/assurrussa/goadmin/models"
)

const testEnrollmentID = "setup-opaque"

type localEnrollmentTestRuntime struct {
	localMFAPublishRuntime
	binding     [32]byte
	result      goauth.LoginResult
	kind        goauth.MFALoginKind
	beginErr    error
	confirmErr  error
	factorCalls int
	setupCalls  int
	onConfirm   func()
}

func (r *localEnrollmentTestRuntime) BeginLocalLogin(
	_ context.Context, _ goauth.LoginRequest, binding [32]byte,
) (goauth.LocalLoginResult, error) {
	r.binding = binding
	return goauth.LocalLoginResult{Pending: &goauth.MFALoginChallenge{
		ID: testLocalMFAChallenge, Kind: r.kind, ExpiresAt: time.Now().Add(time.Minute),
	}}, nil
}

func (r *localEnrollmentTestRuntime) CompleteLocalLogin(
	context.Context, string, [32]byte, string,
) (goauth.LoginResult, error) {
	r.factorCalls++
	return goauth.LoginResult{}, goauth.ErrMFALoginDenied
}

func (r *localEnrollmentTestRuntime) BeginLocalLoginEnrollment(
	_ context.Context, id string, binding [32]byte,
) (goauth.MFAEnrollment, error) {
	r.setupCalls++
	if id != testLocalMFAChallenge || binding != r.binding {
		return goauth.MFAEnrollment{}, goauth.ErrInvalidToken
	}
	if r.beginErr != nil {
		return goauth.MFAEnrollment{}, r.beginErr
	}
	return goauth.MFAEnrollment{
		ID: testEnrollmentID, Secret: "synthetic-setup", URL: "otpauth://totp/synthetic",
		ExpiresAt: time.Now().Add(time.Minute), RecoveryCodes: []string{"synthetic-recovery"},
	}, nil
}

func (r *localEnrollmentTestRuntime) CompleteLocalLoginEnrollment(
	_ context.Context, id, enrollmentID string, binding [32]byte, _ string,
) (goauth.LoginResult, error) {
	if id != testLocalMFAChallenge || enrollmentID != testEnrollmentID || binding != r.binding {
		return goauth.LoginResult{}, goauth.ErrInvalidToken
	}
	if r.confirmErr != nil {
		return goauth.LoginResult{}, r.confirmErr
	}
	if r.onConfirm != nil {
		r.onConfirm()
	}
	return r.result, nil
}

func localEnrollmentFixture(t *testing.T) (*Service, *fiber.App, *localEnrollmentTestRuntime, *http.Cookie) {
	t.Helper()
	subject := goauth.NewSubjectID()
	rt := &localEnrollmentTestRuntime{
		kind: goauth.MFALoginKindEnrollment,
		localMFAPublishRuntime: localMFAPublishRuntime{
			auth: goauth.AuthContext{SubjectID: subject, SessionID: testLocalCanonicalSession, Realm: goauth.RealmAdmin},
		},
		result: goauth.LoginResult{
			Account: goauth.Account{Subject: goauth.Subject{ID: subject}},
			Tokens: goauth.TokenPair{AccessToken: "synthetic", Session: goauth.Session{
				ID: testLocalCanonicalSession, ExpiresAt: time.Now().Add(time.Hour),
			}},
		},
	}
	service := NewService(fibersession.NewStore(), roleServiceStub{},
		WithRuntime(rt, adminRepositoryStub{admin: models.Admin{ID: 1, SubjectID: subject}}),
		WithBrowserState(browserstate.NewMemory()))
	app := fiber.New()
	app.Post("/begin", func(c fiber.Ctx) error {
		_, _, err := service.LoginAdmin(c, "admin@example.test", "password", true)
		require.ErrorIs(t, err, ErrLocalMFARequired)
		return c.SendStatus(http.StatusNoContent)
	})
	response := performSessionRequest(t, app, http.MethodPost, "/begin", nil)
	cookie := requireSessionCookie(t, response)
	require.NoError(t, response.Body.Close())
	return service, app, rt, cookie
}

func TestRequiredSetupRemainsAnonymousUntilConfirmed(t *testing.T) {
	service, app, rt, cookie := localEnrollmentFixture(t)
	app.Get("/inspect", func(c fiber.Ctx) error {
		pending, err := service.PendingLocalMFA(c)
		require.NoError(t, err)
		require.Equal(t, goauth.MFALoginKindEnrollment, pending.Kind)
		admin, err := service.GetAdminAuth(c)
		require.NoError(t, err)
		require.Nil(t, admin)
		return c.SendStatus(http.StatusNoContent)
	})
	app.Post("/setup", func(c fiber.Ctx) error {
		setup, err := service.BeginLocalAdminEnrollment(c, testLocalMFAChallenge)
		require.NoError(t, err)
		require.Equal(t, testEnrollmentID, setup.ID)
		require.Equal(t, []string{"synthetic-recovery"}, setup.RecoveryCodes)
		return c.SendStatus(http.StatusNoContent)
	})
	app.Post("/confirm", func(c fiber.Ctx) error {
		admin, _, err := service.CompleteLocalAdminEnrollment(c, testLocalMFAChallenge, testEnrollmentID, "123456")
		require.NoError(t, err)
		require.Equal(t, int64(1), admin.ID)
		return c.SendStatus(http.StatusNoContent)
	})
	for _, request := range []struct{ method, path string }{
		{http.MethodGet, "/inspect"}, {http.MethodPost, "/setup"}, {http.MethodGet, "/inspect"},
	} {
		response := performSessionRequest(t, app, request.method, request.path, cookie)
		require.NoError(t, response.Body.Close())
	}
	require.Equal(t, 1, rt.setupCalls)
	response := performSessionRequest(t, app, http.MethodPost, "/confirm", cookie)
	authCookie := requireSessionCookie(t, response)
	require.NotEqual(t, cookie.Value, authCookie.Value)
	require.NoError(t, response.Body.Close())
	require.NoError(t, service.checkLocalLoginPublication(t.Context(), "local-login:"+cookie.Value, testLocalCanonicalSession))
	sess, err := service.store.GetByID(t.Context(), authCookie.Value)
	require.NoError(t, err)
	defer sess.Release()
	require.NotNil(t, sess.Get(sessioncore.AuthAdminKey.String()))
}

func TestEnrollmentCannotUseFactorEndpoint(t *testing.T) {
	service, app, rt, cookie := localEnrollmentFixture(t)
	app.Post("/factor", func(c fiber.Ctx) error {
		_, _, err := service.CompleteLocalAdmin(c, testLocalMFAChallenge, "123456")
		require.ErrorIs(t, err, goauth.ErrInvalidToken)
		return c.SendStatus(http.StatusNoContent)
	})
	response := performSessionRequest(t, app, http.MethodPost, "/factor", cookie)
	require.NoError(t, response.Body.Close())
	require.Zero(t, rt.factorCalls)
	record, found, err := service.localLoginStates.Load(t.Context(), "local-login:"+cookie.Value)
	require.NoError(t, err)
	require.True(t, found)
	require.Empty(t, record.Owner)
	require.False(t, record.LocalLoginTerminal)
}

func TestEnrollmentWrongCodeRetriesButUnknownOutcomePinsOwner(t *testing.T) {
	for _, test := range []struct {
		name  string
		cause error
		owned bool
	}{
		{"wrong-code", goauth.ErrMFALoginDenied, false},
		{"rate-limit", goauth.ErrAuthenticationRateLimited, false},
		{"unknown-commit", goauth.ErrOperationOutcomeUnknown, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, app, rt, cookie := localEnrollmentFixture(t)
			rt.confirmErr = test.cause
			app.Post("/confirm", func(c fiber.Ctx) error {
				admin, account, err := service.CompleteLocalAdminEnrollment(
					c, testLocalMFAChallenge, testEnrollmentID, "123456",
				)
				require.ErrorIs(t, err, test.cause)
				require.Zero(t, admin.ID)
				require.True(t, account.Subject.ID.IsZero())
				return c.SendStatus(http.StatusNoContent)
			})
			response := performSessionRequest(t, app, http.MethodPost, "/confirm", cookie)
			require.Empty(t, response.Cookies())
			require.NoError(t, response.Body.Close())
			record, found, err := service.localLoginStates.Load(t.Context(), "local-login:"+cookie.Value)
			require.NoError(t, err)
			require.True(t, found)
			require.Equal(t, test.owned, record.Owner != "")
			require.False(t, record.LocalLoginTerminal)
		})
	}
}

func TestEnrollmentBeginUnknownResultDoesNotExposeSecret(t *testing.T) {
	service, app, rt, cookie := localEnrollmentFixture(t)
	rt.beginErr = goauth.ErrOperationOutcomeUnknown
	app.Post("/setup", func(c fiber.Ctx) error {
		setup, err := service.BeginLocalAdminEnrollment(c, testLocalMFAChallenge)
		require.ErrorIs(t, err, goauth.ErrOperationOutcomeUnknown)
		require.Empty(t, setup.Secret)
		return c.SendStatus(http.StatusNoContent)
	})
	response := performSessionRequest(t, app, http.MethodPost, "/setup", cookie)
	require.NoError(t, response.Body.Close())
	record, _, err := service.localLoginStates.Load(t.Context(), "local-login:"+cookie.Value)
	require.NoError(t, err)
	require.NotEmpty(t, record.Owner)
	require.False(t, record.LocalLoginTerminal)
}

func TestEnrollmentSaveFailureRevokesCanonicalResult(t *testing.T) {
	service, app, rt, cookie := localEnrollmentFixture(t)
	storage := &failingSetStorage{Storage: service.store.Storage, failErr: errors.New("save interrupted")}
	service.store.Storage = storage
	app.Post("/confirm", func(c fiber.Ctx) error {
		_, _, err := service.CompleteLocalAdminEnrollment(c, testLocalMFAChallenge, testEnrollmentID, "123456")
		require.ErrorIs(t, err, storage.failErr)
		return c.SendStatus(http.StatusNoContent)
	})
	response := performSessionRequest(t, app, http.MethodPost, "/confirm", cookie)
	require.Empty(t, response.Cookies())
	require.NoError(t, response.Body.Close())
	require.Equal(t, 1, rt.logoutCount)
}

func TestEnrollmentAbandonmentFencesLateCanonicalResult(t *testing.T) {
	service, app, rt, cookie := localEnrollmentFixture(t)
	app.Get("/login", func(c fiber.Ctx) error {
		require.NoError(t, service.PrepareLocalLogin(c))
		return c.SendStatus(http.StatusNoContent)
	})
	var replacement *http.Cookie
	rt.onConfirm = func() {
		response := performSessionRequest(t, app, http.MethodGet, "/login", cookie)
		replacement = requireSessionCookie(t, response)
		require.NoError(t, response.Body.Close())
	}
	app.Post("/confirm", func(c fiber.Ctx) error {
		_, _, err := service.CompleteLocalAdminEnrollment(c, testLocalMFAChallenge, testEnrollmentID, "123456")
		require.Error(t, err)
		return c.SendStatus(http.StatusConflict)
	})
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/confirm", nil)
	request.AddCookie(cookie)
	response, err := app.Test(request)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, response.StatusCode)
	require.Empty(t, response.Cookies())
	require.NoError(t, response.Body.Close())
	require.NotNil(t, replacement)
	require.NotEqual(t, cookie.Value, replacement.Value)
	require.Equal(t, 1, rt.logoutCount)
	require.Error(t, service.checkLocalLoginPublication(t.Context(), "local-login:"+cookie.Value, testLocalCanonicalSession))
}
