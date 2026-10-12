//nolint:testpackage // Exercises private local-login ownership and browser-session publication fences.
package adminservice

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	fibersession "github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/require"

	sessioncore "github.com/assurrussa/goadmin/infrastructure/core/session"
	internalauth "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/auth/browserstate"
	"github.com/assurrussa/goadmin/models"
)

const (
	testLocalMFAChallenge     = "opaque"
	testLocalCanonicalSession = "canonical"
	testLocalBrowserID        = "browser"
	testLocalSyntheticToken   = "synthetic"
)

type localMFATestRuntime struct {
	runtime
	binding     [32]byte
	completed   bool
	completeErr error
	beginErr    error
}

func (r *localMFATestRuntime) BeginLocalLogin(
	_ context.Context, request goauth.LoginRequest, binding [32]byte,
) (goauth.LocalLoginResult, error) {
	if request.Realm != goauth.RealmAdmin {
		return goauth.LocalLoginResult{}, goauth.ErrMembershipDenied
	}
	if r.beginErr != nil {
		return goauth.LocalLoginResult{}, r.beginErr
	}
	r.binding = binding
	return goauth.LocalLoginResult{Pending: &goauth.MFALoginChallenge{
		ID: testLocalMFAChallenge, ExpiresAt: time.Now().Add(time.Minute),
	}}, nil
}

func (r *localMFATestRuntime) CompleteLocalLogin(
	_ context.Context, id string, binding [32]byte, _ string,
) (goauth.LoginResult, error) {
	r.completed = id == testLocalMFAChallenge && binding == r.binding
	if r.completeErr != nil {
		return goauth.LoginResult{}, r.completeErr
	}
	return goauth.LoginResult{}, goauth.ErrMFALoginDenied
}

func TestLocalMFAPendingNeverAuthenticatesBrowser(t *testing.T) {
	store := fibersession.NewStore()
	rt := &localMFATestRuntime{}
	service := NewService(store, roleServiceStub{},
		WithRuntime(rt, adminRepositoryStub{}), WithBrowserState(browserstate.NewMemory()))
	app := fiber.New()
	app.Post("/begin", func(c fiber.Ctx) error {
		_, _, err := service.LoginAdmin(c, "admin@example.test", "password", true)
		require.ErrorIs(t, err, ErrLocalMFARequired)
		return c.SendStatus(http.StatusNoContent)
	})
	app.Get("/inspect", func(c fiber.Ctx) error {
		auth, err := service.GetAdminAuth(c)
		require.NoError(t, err)
		require.Nil(t, auth)
		sess, err := store.Get(c)
		require.NoError(t, err)
		defer sess.Release()
		require.Nil(t, sess.Get(sessioncore.AuthAdminKey.String()))
		record, found, err := service.localLoginStates.Load(c, "local-login:"+sess.ID())
		require.NoError(t, err)
		require.True(t, found)
		require.True(t, record.LocalLoginPending.Persistent)
		require.NotEqual(t, [32]byte{}, record.LocalLoginPending.Binding)
		return c.SendStatus(http.StatusNoContent)
	})
	app.Post("/complete", func(c fiber.Ctx) error {
		_, _, err := service.CompleteLocalAdmin(c, testLocalMFAChallenge, "wrong")
		require.Error(t, err)
		return c.SendStatus(http.StatusNoContent)
	})
	response := performSessionRequest(t, app, http.MethodPost, "/begin", nil)
	cookie := requireSessionCookie(t, response)
	require.Zero(t, cookie.MaxAge)
	require.NoError(t, response.Body.Close())
	response = performSessionRequest(t, app, http.MethodGet, "/inspect", cookie)
	require.NoError(t, response.Body.Close())
	response = performSessionRequest(t, app, http.MethodPost, "/complete", cookie)
	require.NoError(t, response.Body.Close())
	require.True(t, rt.completed)
}

func TestLocalMFADoesNotFallBackToLegacyRuntime(t *testing.T) {
	service := NewService(fibersession.NewStore(), roleServiceStub{}, WithRuntime(&runtimeStub{}, adminRepositoryStub{}))
	app := fiber.New()
	app.Post("/begin", func(c fiber.Ctx) error {
		_, _, err := service.LoginAdmin(c, "admin@example.test", "password", false)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrLocalMFARequired)
		return c.SendStatus(http.StatusNoContent)
	})
	response := performSessionRequest(t, app, http.MethodPost, "/begin", nil)
	require.NoError(t, response.Body.Close())
}

func TestLocalLoginClaimFencesReplacementAndDelayedCompletion(t *testing.T) {
	journal := browserstate.NewMemory()
	s := NewService(fibersession.NewStore(), roleServiceStub{}, WithBrowserState(journal))
	first, err := s.claimLocalLogin(t.Context(), testLocalBrowserID, "")
	require.NoError(t, err)
	_, err = s.claimLocalLogin(t.Context(), testLocalBrowserID, "")
	require.Error(t, err)
	first.record.LocalLoginPending = &browserstate.LocalLoginPending{ID: "old", ExpiresAt: time.Now().Add(time.Minute)}
	require.NoError(t, s.commitLocalLogin(t.Context(), first, first.record))
	replacement, err := s.claimLocalLogin(t.Context(), testLocalBrowserID, "")
	require.NoError(t, err)
	replacement.record.LocalLoginPending = &browserstate.LocalLoginPending{ID: "new", ExpiresAt: time.Now().Add(time.Minute)}
	require.NoError(t, s.commitLocalLogin(t.Context(), replacement, replacement.record))
	_, err = s.claimLocalLogin(t.Context(), testLocalBrowserID, "old")
	require.Error(t, err)
	completing, err := s.claimLocalLogin(t.Context(), testLocalBrowserID, "new")
	require.NoError(t, err)
	_, err = s.claimLocalLogin(t.Context(), testLocalBrowserID, "")
	require.Error(t, err)
	// Abandonment uses exact version/owner CAS, then old publication must fail.
	canceled := completing.record
	canceled.LocalLoginTerminal = true
	canceled.LocalLoginPending = nil
	ok, err := journal.Complete(t.Context(), completing.key, completing.record.Version, completing.owner, canceled)
	require.NoError(t, err)
	require.True(t, ok)
	completing.record.LocalLoginTerminal = true
	completing.record.LocalLoginSessionID = "late-session"
	require.Error(t, s.commitLocalLogin(t.Context(), completing, completing.record))
	require.Error(t, s.checkLocalLoginPublication(t.Context(), completing.key, "late-session"))
}

func TestLocalLoginExpiredRecreatedKeyRejectsOldOwner(t *testing.T) {
	journal := browserstate.NewMemory()
	s := NewService(fibersession.NewStore(), roleServiceStub{}, WithBrowserState(journal))
	old, err := s.claimLocalLogin(t.Context(), testLocalBrowserID, "")
	require.NoError(t, err)
	// Simulate authoritative TTL cleanup while an old dependency is delayed.
	require.NoError(t, journal.Delete(t.Context(), old.key))
	current, err := s.claimLocalLogin(t.Context(), testLocalBrowserID, "")
	require.NoError(t, err)
	require.NotEqual(t, old.owner, current.owner)
	old.record.LocalLoginTerminal = true
	old.record.LocalLoginSessionID = "late"
	require.Error(t, s.commitLocalLogin(t.Context(), old, old.record))
}

func TestLocalLoginCancelledBudgetNeverPublishes(t *testing.T) {
	journal := browserstate.NewMemory()
	s := NewService(fibersession.NewStore(), roleServiceStub{}, WithBrowserState(journal))
	claim, err := s.claimLocalLogin(t.Context(), testLocalBrowserID, "")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	claim.record.LocalLoginTerminal = true
	claim.record.LocalLoginSessionID = "late"
	require.Error(t, s.commitLocalLogin(ctx, claim, claim.record))
	r, found, err := journal.Load(t.Context(), claim.key)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, claim.owner, r.Owner)
	require.False(t, r.LocalLoginTerminal)
}

type localMFAPublishRuntime struct {
	runtime
	auth        goauth.AuthContext
	logoutCount int
}

func (r *localMFAPublishRuntime) VerifyAccessToken(context.Context, string, bool) (goauth.AuthContext, error) {
	return r.auth, nil
}

func (r *localMFAPublishRuntime) Logout(context.Context, goauth.SubjectID, string) error {
	r.logoutCount++
	return nil
}

func TestLocalLoginSessionSaveFailureRevokesCanonicalResult(t *testing.T) {
	subject := goauth.NewSubjectID()
	result := goauth.LoginResult{
		Account: goauth.Account{Subject: goauth.Subject{ID: subject}},
		Tokens: goauth.TokenPair{AccessToken: testLocalSyntheticToken, Session: goauth.Session{
			ID: testLocalCanonicalSession, ExpiresAt: time.Now().Add(time.Hour),
		}},
	}
	rt := &localMFAPublishRuntime{auth: goauth.AuthContext{
		SubjectID: subject, SessionID: testLocalCanonicalSession, Realm: goauth.RealmAdmin,
	}}
	store := fibersession.NewStore()
	storage := &failingSetStorage{Storage: store.Storage}
	store.Storage = storage
	journal := browserstate.NewMemory()
	s := NewService(store, roleServiceStub{},
		WithRuntime(rt, adminRepositoryStub{admin: models.Admin{ID: 1, SubjectID: subject}}), WithBrowserState(journal))
	claim, err := s.claimLocalLogin(t.Context(), testLocalBrowserID, "")
	require.NoError(t, err)
	storage.failErr = errors.New("interrupted session save")
	app := fiber.New()
	app.Post("/finish", func(c fiber.Ctx) error {
		_, _, err := s.finishClaimedLocalAdmin(c.Context(), c, claim, result, false)
		require.ErrorIs(t, err, storage.failErr)
		return c.SendStatus(http.StatusNoContent)
	})
	response := performSessionRequest(t, app, http.MethodPost, "/finish", nil)
	require.NoError(t, response.Body.Close())
	require.Equal(t, 1, rt.logoutCount)
	_, err = s.claimLocalLogin(t.Context(), testLocalBrowserID, "")
	require.Error(t, err)
}

func TestLocalLoginTerminalPinsExactCanonicalSession(t *testing.T) {
	journal := browserstate.NewMemory()
	s := NewService(fibersession.NewStore(), roleServiceStub{}, WithBrowserState(journal))
	claim, err := s.claimLocalLogin(t.Context(), testLocalBrowserID, "")
	require.NoError(t, err)
	claim.record.LocalLoginTerminal = true
	claim.record.LocalLoginSessionID = testLocalCanonicalSession
	require.NoError(t, s.commitLocalLogin(t.Context(), claim, claim.record))
	require.NoError(t, s.checkLocalLoginPublication(t.Context(), claim.key, testLocalCanonicalSession))
	require.Error(t, s.checkLocalLoginPublication(t.Context(), claim.key, "other"))
	// Explicit cancellation invalidates a cookie response still in flight.
	r, _, err := journal.Load(t.Context(), claim.key)
	require.NoError(t, err)
	ok, err := journal.Claim(t.Context(), claim.key, r.Version, "cancel")
	require.NoError(t, err)
	require.True(t, ok)
	r.LocalLoginSessionID = ""
	ok, err = journal.Complete(t.Context(), claim.key, r.Version, "cancel", r)
	require.NoError(t, err)
	require.True(t, ok)
	require.Error(t, s.checkLocalLoginPublication(t.Context(), claim.key, testLocalCanonicalSession))
}

func withLocalLoginTestState() Option {
	return func(s *Service) error { s.localLoginStates = browserstate.NewMemory(); return nil }
}

type localMFAInterleavingRuntime struct {
	localMFAPublishRuntime
	result  goauth.LoginResult
	entered chan struct{}
	resume  chan struct{}
}

func (r *localMFAInterleavingRuntime) BeginLocalLogin(
	context.Context, goauth.LoginRequest, [32]byte,
) (goauth.LocalLoginResult, error) {
	return goauth.LocalLoginResult{Pending: &goauth.MFALoginChallenge{
		ID: testLocalMFAChallenge, ExpiresAt: time.Now().Add(time.Minute),
	}}, nil
}

func (r *localMFAInterleavingRuntime) CompleteLocalLogin(context.Context, string, [32]byte, string) (goauth.LoginResult, error) {
	close(r.entered)
	<-r.resume
	return r.result, nil
}

func TestLocalLoginAbandonmentFencesInFlightCanonicalCompletion(t *testing.T) {
	subject := goauth.NewSubjectID()
	rt := &localMFAInterleavingRuntime{
		localMFAPublishRuntime: localMFAPublishRuntime{
			auth: goauth.AuthContext{SubjectID: subject, SessionID: testLocalCanonicalSession, Realm: goauth.RealmAdmin},
		},
		result: goauth.LoginResult{
			Account: goauth.Account{Subject: goauth.Subject{ID: subject}},
			Tokens: goauth.TokenPair{AccessToken: testLocalSyntheticToken, Session: goauth.Session{
				ID: testLocalCanonicalSession, ExpiresAt: time.Now().Add(time.Hour),
			}},
		},
		entered: make(chan struct{}), resume: make(chan struct{}),
	}
	var unblock sync.Once
	release := func() { unblock.Do(func() { close(rt.resume) }) }
	t.Cleanup(release)
	journal := browserstate.NewMemory()
	s := NewService(fibersession.NewStore(), roleServiceStub{},
		WithRuntime(rt, adminRepositoryStub{admin: models.Admin{ID: 1, SubjectID: subject}}), WithBrowserState(journal))
	app := fiber.New()
	app.Post("/begin", func(c fiber.Ctx) error {
		_, _, err := s.LoginAdmin(c, "a@example.test", "password", false)
		if !errors.Is(err, ErrLocalMFARequired) {
			return c.SendStatus(500)
		}
		return c.SendStatus(204)
	})
	app.Post("/complete", func(c fiber.Ctx) error {
		_, _, err := s.CompleteLocalAdmin(c, testLocalMFAChallenge, "123456")
		if err != nil {
			return c.SendStatus(409)
		}
		return c.SendStatus(204)
	})
	app.Get("/login", func(c fiber.Ctx) error {
		if err := s.PrepareLocalLogin(c); err != nil {
			return err
		}
		return c.SendStatus(204)
	})
	response := performSessionRequest(t, app, http.MethodPost, "/begin", nil)
	cookie := requireSessionCookie(t, response)
	require.NoError(t, response.Body.Close())
	type outcome struct {
		status int
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/complete", nil)
		request.AddCookie(cookie)
		response, err := app.Test(request, fiber.TestConfig{Timeout: 5 * time.Second, FailOnTimeout: true})
		result := outcome{err: err}
		if response != nil {
			result.status = response.StatusCode
			result.err = errors.Join(result.err, response.Body.Close())
		}
		done <- result
	}()
	select {
	case <-rt.entered:
	case <-time.After(time.Second):
		t.Fatal("completion did not enter Runtime")
	}
	response = performSessionRequest(t, app, http.MethodGet, "/login", cookie)
	fresh := requireSessionCookie(t, response)
	require.NotEqual(t, cookie.Value, fresh.Value)
	require.NoError(t, response.Body.Close())
	release()
	finished := <-done
	require.NoError(t, finished.err)
	require.Equal(t, 409, finished.status)
	require.Equal(t, 1, rt.logoutCount)
	require.Error(t, s.checkLocalLoginPublication(t.Context(), "local-login:"+cookie.Value, testLocalCanonicalSession))
}

func TestLocalMFADenialReleasesButUnknownOutcomeRetainsOwner(t *testing.T) {
	for _, test := range []struct {
		name  string
		cause error
		owned bool
	}{
		{"wrong-code", goauth.ErrMFALoginDenied, false},
		{"rate-limit", goauth.ErrAuthenticationRateLimited, false},
		{"unknown", goauth.ErrOperationOutcomeUnknown, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			rt := &localMFATestRuntime{completeErr: test.cause}
			journal := browserstate.NewMemory()
			s := NewService(fibersession.NewStore(), roleServiceStub{}, WithRuntime(rt, adminRepositoryStub{}), WithBrowserState(journal))
			app := fiber.New()
			app.Post("/begin", func(c fiber.Ctx) error {
				_, _, err := s.LoginAdmin(c, "a@example.test", "password", false)
				require.ErrorIs(t, err, ErrLocalMFARequired)
				return c.SendStatus(204)
			})
			app.Post("/complete", func(c fiber.Ctx) error {
				_, _, err := s.CompleteLocalAdmin(c, testLocalMFAChallenge, "code")
				require.ErrorIs(t, err, test.cause)
				return c.SendStatus(204)
			})
			response := performSessionRequest(t, app, http.MethodPost, "/begin", nil)
			cookie := requireSessionCookie(t, response)
			require.NoError(t, response.Body.Close())
			response = performSessionRequest(t, app, http.MethodPost, "/complete", cookie)
			require.NoError(t, response.Body.Close())
			record, found, err := journal.Load(t.Context(), "local-login:"+cookie.Value)
			require.NoError(t, err)
			require.True(t, found)
			require.Equal(t, test.owned, record.Owner != "")
			require.Equal(t, testLocalMFAChallenge, record.LocalLoginPending.ID)
		})
	}
}

type unavailableLocalLoginJournal struct {
	browserstate.Store
	cause error
}

func (s unavailableLocalLoginJournal) Load(context.Context, string) (browserstate.Record, bool, error) {
	return browserstate.Record{}, false, s.cause
}

func TestLocalLoginTransientJournalReadPreservesBrowserCredentials(t *testing.T) {
	store := fibersession.NewStore()
	app, cookie := seedAdminBrowserSession(t, store)
	saved, err := store.GetByID(t.Context(), cookie.Value)
	require.NoError(t, err)
	state, ok := saved.Get(sessioncore.AuthAdminKey.String()).(*browserSession)
	require.True(t, ok)
	require.NotNil(t, state)
	state.LocalLoginFlowKey = "local-login:original"
	saved.Set(sessioncore.AuthAdminKey.String(), state)
	require.NoError(t, saved.Save())
	saved.Release()
	want := errors.New("journal unavailable")
	s := NewService(store, roleServiceStub{}, WithRuntime(&runtimeStub{}, adminRepositoryStub{}))
	s.localLoginStates = unavailableLocalLoginJournal{Store: browserstate.NewMemory(), cause: want}
	app.Get("/auth", func(c fiber.Ctx) error {
		_, err := s.GetAdminAuth(c)
		require.ErrorIs(t, err, want)
		return c.SendStatus(204)
	})
	response := performSessionRequest(t, app, http.MethodGet, "/auth", cookie)
	require.NoError(t, response.Body.Close())
	saved, err = store.GetByID(t.Context(), cookie.Value)
	require.NoError(t, err)
	defer saved.Release()
	require.NotNil(t, saved.Get(sessioncore.AuthAdminKey.String()))
}

func TestExternalSessionSkipsLocalMFAManagement(t *testing.T) {
	f := newExternalFixture(t)
	cookie := admittedExternal(t, f)
	f.app.Get("/mfa-status", func(c fiber.Ctx) error {
		_, err := f.service.LocalMFAStatus(c)
		require.ErrorIs(t, err, goauth.ErrSessionRevoked)
		admin, err := f.service.GetAdminAuth(c)
		require.NoError(t, err)
		require.NotNil(t, admin)
		return c.SendStatus(http.StatusNoContent)
	})
	response := performSessionRequest(t, f.app, http.MethodGet, "/mfa-status", cookie)
	require.NoError(t, response.Body.Close())
}

func TestLocalLoginRuntimeRejectionDoesNotPublishAnonymousCookie(t *testing.T) {
	for _, cause := range []error{goauth.ErrInvalidCredentials, goauth.ErrMembershipDenied} {
		t.Run(cause.Error(), func(t *testing.T) {
			rt := &localMFATestRuntime{beginErr: cause}
			s := NewService(fibersession.NewStore(), roleServiceStub{},
				WithRuntime(rt, adminRepositoryStub{}), WithBrowserState(browserstate.NewMemory()))
			app := fiber.New()
			app.Post("/begin", func(c fiber.Ctx) error {
				_, _, err := s.LoginAdmin(c, "admin@example.test", "password", false)
				require.ErrorIs(t, err, cause)
				return c.SendStatus(http.StatusNoContent)
			})
			response := performSessionRequest(t, app, http.MethodPost, "/begin", nil)
			defer response.Body.Close()
			for _, cookie := range response.Cookies() {
				require.Empty(t, cookie.Value, "rejected login must not publish a browser credential")
			}
		})
	}
}

type localLoginDeadlineRoles struct {
	roleServiceStub
	deadline time.Time
}

func (r *localLoginDeadlineRoles) GetRolesAdmin(ctx context.Context, _ int64) ([]internalauth.Role, error) {
	r.deadline, _ = ctx.Deadline()
	return nil, nil
}

type localLoginDeadlineStorage struct {
	fiber.Storage
	savedID  string
	deadline time.Time
	cancel   context.CancelFunc
}

func (s *localLoginDeadlineStorage) SetWithContext(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	s.savedID = key
	s.deadline, _ = ctx.Deadline()
	err := s.Storage.SetWithContext(ctx, key, value, ttl)
	if s.cancel != nil {
		cancel := s.cancel
		s.cancel = nil
		cancel()
	}
	return err
}

type localLoginCleanupRuntime struct {
	localMFAPublishRuntime
	cleanupErr      error
	cleanupDeadline time.Time
}

func (r *localLoginCleanupRuntime) Logout(ctx context.Context, _ goauth.SubjectID, _ string) error {
	r.logoutCount++
	r.cleanupErr = ctx.Err()
	r.cleanupDeadline, _ = ctx.Deadline()
	return r.cleanupErr
}

func TestLocalLoginPublicationUsesBudgetAndBoundedCompensation(t *testing.T) {
	for _, cancelAfterSave := range []bool{false, true} {
		name := "publish"
		if cancelAfterSave {
			name = "cancel-after-save"
		}
		t.Run(name, func(t *testing.T) {
			subject := goauth.NewSubjectID()
			result := goauth.LoginResult{
				Account: goauth.Account{Subject: goauth.Subject{ID: subject}},
				Tokens: goauth.TokenPair{AccessToken: testLocalSyntheticToken, Session: goauth.Session{
					ID: testLocalCanonicalSession, ExpiresAt: time.Now().Add(time.Hour),
				}},
			}
			rt := &localLoginCleanupRuntime{localMFAPublishRuntime: localMFAPublishRuntime{
				auth: goauth.AuthContext{SubjectID: subject, SessionID: testLocalCanonicalSession, Realm: goauth.RealmAdmin},
			}}
			store := fibersession.NewStore()
			storage := &localLoginDeadlineStorage{Storage: store.Storage}
			store.Storage = storage
			roles := &localLoginDeadlineRoles{}
			s := NewService(store, roles,
				WithRuntime(rt, adminRepositoryStub{admin: models.Admin{ID: 1, SubjectID: subject}}),
				WithBrowserState(browserstate.NewMemory()))
			claim, err := s.claimLocalLogin(t.Context(), testLocalBrowserID, "")
			require.NoError(t, err)
			var deadline time.Time
			app := fiber.New()
			app.Post("/finish", func(c fiber.Ctx) error {
				ctx, cancel := context.WithTimeout(c.Context(), time.Minute)
				defer cancel()
				deadline, _ = ctx.Deadline()
				if cancelAfterSave {
					storage.cancel = cancel
				}
				_, _, err := s.finishClaimedLocalAdmin(ctx, c, claim, result, false)
				if cancelAfterSave {
					require.ErrorIs(t, err, context.Canceled)
				} else {
					require.NoError(t, err)
				}
				return c.SendStatus(http.StatusNoContent)
			})
			response := performSessionRequest(t, app, http.MethodPost, "/finish", nil)
			defer response.Body.Close()
			require.Equal(t, deadline, roles.deadline, "role lookup must inherit the operation budget")
			if cancelAfterSave {
				require.Positive(t, rt.logoutCount)
				require.NoError(t, rt.cleanupErr, "compensation must survive operation cancellation")
				require.False(t, rt.cleanupDeadline.IsZero())
				require.True(t, rt.cleanupDeadline.Before(deadline), "compensation has its own shorter budget")
				require.Empty(t, response.Cookies(), "late failure must not publish or clear a newer browser cookie")
				require.NotEmpty(t, storage.savedID)
				_, found, err := s.states.Load(t.Context(), storage.savedID)
				require.NoError(t, err)
				require.False(t, found, "compensation must delete the exact rotated auth journal")
				raw, err := storage.GetWithContext(t.Context(), storage.savedID)
				require.NoError(t, err)
				require.Empty(t, raw, "compensation must delete the exact rotated Fiber projection")
			} else {
				require.Equal(t, deadline, storage.deadline, "session save must inherit the operation budget")
				require.Zero(t, rt.logoutCount)
				require.NotEmpty(t, requireSessionCookie(t, response).Value)
			}
		})
	}
}

type lateLocalLoginJournal struct {
	browserstate.Store
	cancelComplete context.CancelFunc
	cancelLoad     context.CancelFunc
}

func (s *lateLocalLoginJournal) Complete(
	ctx context.Context, id string, version int64, owner string, record browserstate.Record,
) (bool, error) {
	ok, err := s.Store.Complete(ctx, id, version, owner, record)
	if s.cancelComplete != nil {
		s.cancelComplete()
	}
	return ok, err
}

func (s *lateLocalLoginJournal) Load(ctx context.Context, id string) (browserstate.Record, bool, error) {
	record, found, err := s.Store.Load(ctx, id)
	if s.cancelLoad != nil {
		s.cancelLoad()
	}
	return record, found, err
}

func TestLocalLoginLateJournalSuccessCannotPublish(t *testing.T) {
	for _, duringCommit := range []bool{false, true} {
		name := "publication-read"
		if duringCommit {
			name = "terminal-commit"
		}
		t.Run(name, func(t *testing.T) {
			journal := &lateLocalLoginJournal{Store: browserstate.NewMemory()}
			s := NewService(fibersession.NewStore(), roleServiceStub{}, WithBrowserState(journal))
			claim, err := s.claimLocalLogin(t.Context(), testLocalBrowserID, "")
			require.NoError(t, err)
			claim.record.LocalLoginTerminal = true
			claim.record.LocalLoginSessionID = testLocalCanonicalSession
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if duringCommit {
				journal.cancelComplete = cancel
				require.ErrorIs(t, s.commitLocalLogin(ctx, claim, claim.record), context.Canceled)
			} else {
				require.NoError(t, s.commitLocalLogin(ctx, claim, claim.record))
				journal.cancelLoad = cancel
				require.ErrorIs(t, s.checkLocalLoginPublication(ctx, claim.key, testLocalCanonicalSession), context.Canceled)
			}
		})
	}
}
