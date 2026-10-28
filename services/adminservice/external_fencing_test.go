//nolint:testpackage // Exercises the journal owner before provider interaction.
package adminservice

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	sessioncore "github.com/assurrussa/goadmin/infrastructure/core/session"
	"github.com/assurrussa/goadmin/internal/auth/browserstate"
)

type cleanupInterleaveStore struct {
	browserstate.Store
	beforeDelete func()
}

func (s cleanupInterleaveStore) DeleteIfCurrent(ctx context.Context, id string, version int64, owner string) (bool, error) {
	s.beforeDelete()
	return s.Store.DeleteIfCurrent(ctx, id, version, owner)
}

func TestStaleTerminalValidationReturnsRetryWhenCleanupLosesCAS(t *testing.T) {
	f := newExternalFixture(t)
	cookie := admittedExternal(t, f)
	f.authority.err = goauth.ErrSessionRevoked
	f.service.states = cleanupInterleaveStore{Store: f.service.states, beforeDelete: func() {
		changeExternalRecoveryRecord(t, f, cookie, func(*browserstate.Record) {})
	}}
	f.app.Get("/stale-terminal", func(c fiber.Ctx) error {
		_, err := f.service.GetAdminAuth(c)
		require.ErrorIs(t, err, ErrRefreshRetryRequired)
		require.NotErrorIs(t, err, goauth.ErrSessionRevoked, "stale terminal evidence must not stop the newer login")
		return c.SendStatus(http.StatusServiceUnavailable)
	})
	response, err := f.app.Test(requestWithCookie(http.MethodGet, "/stale-terminal", cookie))
	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, response.StatusCode)
	require.Empty(t, response.Cookies())
	_ = response.Body.Close()
	_ = mustRecoveryRecord(t, f, cookie)
}

func TestOwnedRecoveryCapturesOwnerBeforeProviderAndDoesNotNestClaim(t *testing.T) {
	f := newExternalFixture(t)
	cookie := admittedExternal(t, f)
	before := mustRecoveryRecord(t, f, cookie)
	f.app.Get("/owned-recover", func(c fiber.Ctx) error {
		snapshot, err := f.service.BeginOwnedExternalAdminRecovery(c)
		require.NoError(t, err)
		current := mustRecoveryRecord(t, f, cookie)
		require.NotEmpty(t, current.Owner, "owner must exist before host SDK can Verify/Renew")
		_, err = f.service.BeginOwnedExternalAdminRecovery(c)
		require.ErrorIs(t, err, ErrRefreshInProgress)
		fresh := freshRecoveryBinding(f, snapshot.Binding())
		require.NoError(t, f.service.CompleteExternalAdminRecovery(c, snapshot, fresh))
		current = mustRecoveryRecord(t, f, cookie)
		require.Equal(t, before.Version+1, current.Version, "completion uses its existing claim")
		require.Empty(t, current.Owner)
		return c.SendStatus(http.StatusNoContent)
	})
	response := performSessionRequest(t, f.app, http.MethodGet, "/owned-recover", cookie)
	require.Empty(t, response.Cookies(), "passive recovery must not deliver browser credentials")
	_ = response.Body.Close()
}

func TestOwnedRecoveryCancellationAndUnchangedProofRetainVersion(t *testing.T) {
	for _, unchanged := range []bool{false, true} {
		t.Run(map[bool]string{false: "provider-error", true: "unchanged-proof"}[unchanged], func(t *testing.T) {
			f := newExternalFixture(t)
			cookie := admittedExternal(t, f)
			before := mustRecoveryRecord(t, f, cookie)
			f.app.Get("/cancel-recover", func(c fiber.Ctx) error {
				snapshot, err := f.service.BeginOwnedExternalAdminRecovery(c)
				require.NoError(t, err)
				if unchanged {
					require.NoError(t, f.service.CompleteExternalAdminRecovery(c, snapshot, snapshot.Binding()))
				} else {
					require.NoError(t, f.service.CancelExternalAdminRecovery(c, snapshot))
				}
				current := mustRecoveryRecord(t, f, cookie)
				require.Equal(t, before.Version, current.Version)
				require.Empty(t, current.Owner)
				require.Equal(t, before.ExpiresAt, current.ExpiresAt)
				return c.SendStatus(http.StatusNoContent)
			})
			response := performSessionRequest(t, f.app, http.MethodGet, "/cancel-recover", cookie)
			_ = response.Body.Close()
		})
	}
}

func TestStaleTerminalCleanupCannotDeleteNewerAuthorityOrExpireCookie(t *testing.T) {
	f := newExternalFixture(t)
	cookie := admittedExternal(t, f)
	f.app.Get("/stale-cleanup", func(c fiber.Ctx) error {
		sess, err := f.service.store.Get(c)
		require.NoError(t, err)
		defer sess.Release()
		state, ok := sess.Get(sessioncore.AuthAdminKey.String()).(*browserSession)
		require.True(t, ok)
		active, err := f.service.loadBrowserAuthority(c, sess, state)
		require.NoError(t, err)
		require.True(t, active)
		staleVersion := state.Version
		changeExternalRecoveryRecord(t, f, cookie, func(*browserstate.Record) {})
		err = f.service.clearAdminBrowserSession(c, sess)
		require.ErrorIs(t, err, ErrRefreshRetryRequired)
		current := mustRecoveryRecord(t, f, cookie)
		require.Equal(t, staleVersion+1, current.Version)
		return c.SendStatus(http.StatusNoContent)
	})
	response := performSessionRequest(t, f.app, http.MethodGet, "/stale-cleanup", cookie)
	require.Empty(t, response.Cookies())
	_ = response.Body.Close()
}

func TestTransientExternalFailurePreservesJournalAndCookie(t *testing.T) {
	f := newExternalFixture(t)
	cookie := admittedExternal(t, f)
	before := mustRecoveryRecord(t, f, cookie)
	outage := errors.New("synthetic provider unavailable")
	f.authority.err = outage
	f.app.Get("/provider-outage", func(c fiber.Ctx) error {
		_, err := f.service.GetAdminAuth(c)
		require.ErrorIs(t, err, outage)
		require.ErrorIs(t, err, ErrRefreshRetryRequired)
		require.NotErrorIs(t, err, goauth.ErrSessionRevoked)
		return c.SendStatus(http.StatusServiceUnavailable)
	})
	response, err := f.app.Test(requestWithCookie(http.MethodGet, "/provider-outage", cookie))
	require.NoError(t, err)
	require.Empty(t, response.Cookies())
	_ = response.Body.Close()
	current := mustRecoveryRecord(t, f, cookie)
	require.Equal(t, before.Version, current.Version)
	require.WithinDuration(t, before.ExpiresAt, current.ExpiresAt, time.Nanosecond)
}
