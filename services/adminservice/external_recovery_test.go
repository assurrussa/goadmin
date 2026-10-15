//nolint:testpackage,lll // Security cases exercise the private journal and its CAS boundary.
package adminservice

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/internal/auth/browserstate"
	"github.com/assurrussa/goadmin/internal/refreshpolicy"
)

func changeExternalRecoveryRecord(t *testing.T, f externalFixture, cookie *http.Cookie, change func(*browserstate.Record)) {
	t.Helper()
	record, found, err := f.service.states.Load(t.Context(), cookie.Value)
	require.NoError(t, err)
	require.True(t, found)
	if record.External != nil {
		binding := *record.External
		record.External = &binding
	}
	owner := refreshpolicy.Owner("refresh", "test-mutation", time.Now())
	ok, err := f.service.states.Claim(t.Context(), cookie.Value, record.Version, owner)
	require.NoError(t, err)
	require.True(t, ok)
	change(&record)
	ok, err = f.service.states.Complete(t.Context(), cookie.Value, record.Version, owner, record)
	require.NoError(t, err)
	require.True(t, ok)
}

func freshRecoveryBinding(f externalFixture, old ExternalBinding) ExternalBinding {
	now := time.Now()
	old.Generation++
	old.Proof = ExternalProof{IssuedAt: now.Add(-time.Millisecond), ExpiresAt: now.Add(time.Hour)}
	old.Deadline = externalDeadline(old.Proof, old.AbsoluteUntil)
	f.authority.proof = old.Proof
	return old
}

func TestExternalRecoveryPreservesCanonicalSessionAndRejectsReplay(t *testing.T) {
	f := newExternalFixture(t)
	cookie := admittedExternal(t, f)
	before, found, err := f.service.states.Load(t.Context(), cookie.Value)
	require.NoError(t, err)
	require.True(t, found)
	changeExternalRecoveryRecord(t, f, cookie, func(r *browserstate.Record) {
		r.External.Deadline = time.Now().Add(-time.Second)
	})
	// Expiry is a denial, never a protected-handler authorization. It must
	// retain the native CAS fence for the subsequent explicit recovery.
	denied, err := f.app.Test(requestWithCookie(http.MethodGet, "/auth", cookie))
	require.NoError(t, err)
	require.GreaterOrEqual(t, denied.StatusCode, 400)
	_ = denied.Body.Close()
	retained, found, err := f.service.states.Load(t.Context(), cookie.Value)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, before.Tokens.Session.ID, retained.Tokens.Session.ID)

	f.app.Get("/recover", func(c fiber.Ctx) error {
		snapshot, err := f.service.BeginExternalAdminRecovery(c)
		require.NoError(t, err)
		require.NotNil(t, snapshot)
		copied := snapshot.Binding()
		copied.Subject = "modified-copy"
		require.NotEqual(t, copied.Subject, snapshot.Binding().Subject)
		fresh := freshRecoveryBinding(f, snapshot.Binding())
		require.NoError(t, f.service.CompleteExternalAdminRecovery(c, snapshot, fresh))
		require.Error(t, f.service.CompleteExternalAdminRecovery(c, snapshot, fresh), "snapshot replay must lose its version fence")
		return c.SendStatus(http.StatusNoContent)
	})
	response := performSessionRequest(t, f.app, http.MethodGet, "/recover", cookie)
	_ = response.Body.Close()
	for _, delivered := range response.Cookies() {
		if delivered.Name == cookie.Name {
			require.Equal(t, cookie.Value, delivered.Value)
		}
	}
	after, found, err := f.service.states.Load(t.Context(), cookie.Value)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, before.SubjectID, after.SubjectID)
	require.Equal(t, before.Tokens.Session.ID, after.Tokens.Session.ID)
	require.Equal(t, before.Tokens.Session.ExpiresAt, after.Tokens.Session.ExpiresAt)
	require.Equal(t, before.External.AbsoluteUntil, after.External.AbsoluteUntil)
	require.Greater(t, after.Version, retained.Version)
	authenticated := performSessionRequest(t, f.app, http.MethodGet, "/auth", cookie)
	_ = authenticated.Body.Close()
}

func TestExternalRecoveryRejectsLineageChanges(t *testing.T) {
	cases := map[string]func(*ExternalBinding){
		"issuer":                       func(b *ExternalBinding) { b.Issuer += "/other" },
		"subject":                      func(b *ExternalBinding) { b.Subject += "-other" },
		"link":                         func(b *ExternalBinding) { b.LinkID += "-other" },
		"authority":                    func(b *ExternalBinding) { b.AuthorityID += "-other" },
		"provider-session":             func(b *ExternalBinding) { b.ProviderSessionID += "-other" },
		"client":                       func(b *ExternalBinding) { b.ClientID += "-other" },
		"project":                      func(b *ExternalBinding) { b.ProjectID += "-other" },
		"auth-time":                    func(b *ExternalBinding) { b.ExternalAuthTime = b.ExternalAuthTime.Add(time.Second) },
		"login":                        func(b *ExternalBinding) { b.LoginGeneration++ },
		"generation":                   func(b *ExternalBinding) { b.Generation = 0 },
		"absolute-extension":           func(b *ExternalBinding) { b.AbsoluteUntil = b.AbsoluteUntil.Add(time.Second) },
		"expired-absolute":             func(b *ExternalBinding) { b.AbsoluteUntil = time.Now().Add(-time.Second) },
		"same-generation-proof-change": func(b *ExternalBinding) { b.Generation-- },
		"expired-proof":                func(b *ExternalBinding) { b.Deadline = time.Now().Add(-time.Second) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := newExternalFixture(t)
			cookie := admittedExternal(t, f)
			f.app.Get("/recover", func(c fiber.Ctx) error {
				snapshot, err := f.service.BeginExternalAdminRecovery(c)
				require.NoError(t, err)
				fresh := freshRecoveryBinding(f, snapshot.Binding())
				mutate(&fresh)
				require.Error(t, f.service.CompleteExternalAdminRecovery(c, snapshot, fresh))
				return c.SendStatus(http.StatusNoContent)
			})
			response := performSessionRequest(t, f.app, http.MethodGet, "/recover", cookie)
			_ = response.Body.Close()
		})
	}
}

func TestExternalRecoveryRejectsChangesDuringProviderVerification(t *testing.T) {
	for _, change := range []string{"native-logout", "native-revocation", "version", "membership", "link", "legacy", "absolute"} {
		t.Run(change, func(t *testing.T) {
			f := newExternalFixture(t)
			cookie := admittedExternal(t, f)
			f.app.Get("/recover", func(c fiber.Ctx) error {
				snapshot, err := f.service.BeginExternalAdminRecovery(c)
				require.NoError(t, err)
				fresh := freshRecoveryBinding(f, snapshot.Binding())
				switch change {
				case "native-logout":
					require.NoError(t, f.service.states.Delete(c, cookie.Value))
				case "native-revocation":
					require.NoError(t, f.runtime.Runtime.Logout(c, snapshot.record.SubjectID, snapshot.record.Tokens.Session.ID))
				case "version":
					changeExternalRecoveryRecord(t, f, cookie, func(*browserstate.Record) {})
				case "membership":
					f.service.admins = adminRepositoryStub{}
				case "link":
					f.resolver.replacement = true
				case "legacy":
					changeExternalRecoveryRecord(t, f, cookie, func(r *browserstate.Record) { r.External = nil })
				case "absolute":
					snapshot.binding.AbsoluteUntil = time.Now().Add(-time.Second)
				}
				require.Error(t, f.service.CompleteExternalAdminRecovery(c, snapshot, fresh))
				return c.SendStatus(http.StatusNoContent)
			})
			response := performSessionRequest(t, f.app, http.MethodGet, "/recover", cookie)
			_ = response.Body.Close()
		})
	}
}

type deleteBeforeRecoveryCommit struct{ browserstate.Store }

func (s deleteBeforeRecoveryCommit) Complete(ctx context.Context, id string, version int64, owner string, record browserstate.Record) (bool, error) {
	if err := s.Store.Delete(ctx, id); err != nil {
		return false, err
	}
	return s.Store.Complete(ctx, id, version, owner, record)
}

func TestExternalRecoveryCASCannotResurrectDeletedJournal(t *testing.T) {
	f := newExternalFixture(t)
	cookie := admittedExternal(t, f)
	f.service.states = deleteBeforeRecoveryCommit{Store: f.service.states}
	f.app.Get("/recover", func(c fiber.Ctx) error {
		snapshot, err := f.service.BeginExternalAdminRecovery(c)
		require.NoError(t, err)
		require.ErrorIs(t, f.service.CompleteExternalAdminRecovery(c, snapshot, freshRecoveryBinding(f, snapshot.Binding())), goauth.ErrOperationOutcomeUnknown)
		_, found, err := f.service.states.Load(c, cookie.Value)
		require.NoError(t, err)
		require.False(t, found)
		return c.SendStatus(http.StatusNoContent)
	})
	response := performSessionRequest(t, f.app, http.MethodGet, "/recover", cookie)
	_ = response.Body.Close()
}

func TestExternalRecoveryRequiresJournalBinding(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "absolute-expired", true: "legacy"}[legacy], func(t *testing.T) {
			f := newExternalFixture(t)
			cookie := admittedExternal(t, f)
			changeExternalRecoveryRecord(t, f, cookie, func(r *browserstate.Record) {
				if legacy {
					r.External = nil
				} else {
					r.External.AbsoluteUntil = time.Now().Add(-time.Second)
				}
			})
			f.app.Get("/recover", func(c fiber.Ctx) error {
				_, err := f.service.BeginExternalAdminRecovery(c)
				require.ErrorIs(t, err, goauth.ErrSessionRevoked)
				return c.SendStatus(http.StatusNoContent)
			})
			response := performSessionRequest(t, f.app, http.MethodGet, "/recover", cookie)
			_ = response.Body.Close()
		})
	}
}

type recoveryRefreshRuntime struct {
	runtime
	refreshes int
}

func (r *recoveryRefreshRuntime) Refresh(ctx context.Context, token string) (goauth.TokenPair, error) {
	r.refreshes++
	return r.runtime.Refresh(ctx, token)
}

func TestExternalRecoveryRefreshesNativeTokenOnlyAfterFreshProof(t *testing.T) {
	for _, valid := range []bool{false, true} {
		t.Run(map[bool]string{false: "stale-proof", true: "fresh-proof"}[valid], func(t *testing.T) {
			var clock atomic.Int64
			clock.Store(time.Now().UTC().Truncate(time.Second).UnixNano())
			f := newExternalFixture(t, func(c *goauth.Config) {
				c.AccessTTL = time.Second
				c.Now = func() time.Time { return time.Unix(0, clock.Load()) }
			})
			cookie := admittedExternal(t, f)
			clock.Add(int64(2 * time.Second))
			record, found, err := f.service.states.Load(t.Context(), cookie.Value)
			require.NoError(t, err)
			require.True(t, found)
			_, err = f.runtime.Runtime.VerifyAccessToken(t.Context(), record.Tokens.AccessToken, true)
			require.ErrorIs(t, err, goauth.ErrExpiredToken)
			rt := &recoveryRefreshRuntime{runtime: f.service.runtime}
			f.service.runtime = rt
			f.app.Get("/recover", func(c fiber.Ctx) error {
				snapshot, err := f.service.BeginExternalAdminRecovery(c)
				require.NoError(t, err)
				fresh := freshRecoveryBinding(f, snapshot.Binding())
				if !valid {
					fresh.Deadline = time.Now().Add(-time.Second)
				}
				err = f.service.CompleteExternalAdminRecovery(c, snapshot, fresh)
				if valid {
					require.NoError(t, err)
					require.Equal(t, 1, rt.refreshes)
					record, found, loadErr := f.service.states.Load(c, cookie.Value)
					require.NoError(t, loadErr)
					require.True(t, found)
					require.Equal(t, snapshot.record.Tokens.Session.ID, record.Tokens.Session.ID)
				} else {
					require.Error(t, err)
					require.Zero(t, rt.refreshes)
				}
				return c.SendStatus(http.StatusNoContent)
			})
			response := performSessionRequest(t, f.app, http.MethodGet, "/recover", cookie)
			_ = response.Body.Close()
		})
	}
}

func TestExternalRecoverySnapshotSupportsMutationPreflight(t *testing.T) {
	f := newExternalFixture(t)
	cookie := admittedExternal(t, f)
	f.app.Post("/preflight", func(c fiber.Ctx) error {
		snapshot, err := f.service.BeginExternalAdminRecovery(c)
		require.NoError(t, err)
		require.NotNil(t, snapshot)
		// Reading eligibility on a rejected mutation cannot complete recovery.
		require.ErrorIs(t, f.service.CompleteExternalAdminRecovery(c, snapshot, freshRecoveryBinding(f, snapshot.Binding())), goauth.ErrSessionRevoked)
		return c.SendStatus(http.StatusNoContent)
	})
	response := performSessionRequest(t, f.app, http.MethodPost, "/preflight", cookie)
	_ = response.Body.Close()
}
