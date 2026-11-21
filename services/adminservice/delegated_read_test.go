//nolint:testpackage // Exercises the canonical login pin and projected browser state.
package adminservice

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/testkit"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessioncore "github.com/assurrussa/goadmin/infrastructure/core/session"
	auth "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/auth/browserstate"
	"github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
)

type delegatedPreview struct{ permitted bool }

type delegatedReadMember struct {
	admin models.Admin
	err   error
}

func (m *delegatedReadMember) GetBySubjectID(_ context.Context, subjectID goauth.SubjectID) (models.Admin, error) {
	if m.err != nil || m.admin.SubjectID != subjectID {
		return models.Admin{}, m.err // The PostgreSQL repository returns a zero admin on no rows.
	}
	return m.admin, nil
}

func (*delegatedPreview) GetRole(context.Context, int64) (*auth.Role, error) {
	return &auth.Role{ID: 2, Name: "Viewer"}, nil
}

func (p *delegatedPreview) ListRolePermissions(context.Context, int64) ([]auth.Permission, error) {
	if p.permitted {
		return []auth.Permission{{Domain: "product_events", Action: auth.PermissionActionRead}}, nil
	}
	return nil, nil
}

func TestDelegatedReadSessionLifecycle(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	fixture, err := testkit.NewRuntime(func(cfg *goauth.Config) {
		cfg.AccessTTL = time.Second
		cfg.Now = func() time.Time { return now }
	})
	require.NoError(t, err)
	account, err := fixture.Runtime.ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
		Email: "delegated-read@example.test", Password: testAdminPassword,
	})
	require.NoError(t, err)
	admin := models.Admin{ID: 21, SubjectID: account.Subject.ID, UUID: identity.NewUserID()}
	member := &delegatedReadMember{admin: admin}
	roles := &delegatedPreview{}
	isOwner := true
	store := session.NewStore()
	journal := browserstate.NewMemory()
	service := NewService(store, roleServiceStub{}, WithRuntime(fixture.Runtime, member),
		WithBrowserState(journal),
		WithDelegatedReadPolicy(func(context.Context, int64) bool { return isOwner }, roles))
	service.identityLinkSessions = fixture.Store

	app := fiber.New()
	var pin ReadSessionBinding
	app.Post("/login", func(c fiber.Ctx) error {
		_, _, loginErr := service.LoginAdmin(c, account.PrimaryEmail.DisplayValue, testAdminPassword, false)
		require.NoError(t, loginErr)
		return c.SendStatus(http.StatusNoContent)
	})
	app.Post("/approve", func(c fiber.Ctx) error {
		operator, authErr := service.GetAdminAuth(c)
		require.NoError(t, authErr)
		var bindErr error
		pin, bindErr = service.BindDelegatedRead(c.Context(), operator)
		require.NoError(t, bindErr)
		return c.SendStatus(http.StatusNoContent)
	})
	app.Get("/me", func(c fiber.Ctx) error {
		_, authErr := service.GetAdminAuth(c)
		require.NoError(t, authErr)
		return c.SendStatus(http.StatusNoContent)
	})
	app.Post("/rotate-browser-id", func(c fiber.Ctx) error {
		id, rotateErr := service.RotateAdminAuth(c, admin)
		require.NoError(t, rotateErr)
		service.writeSessionCookie(c, id, false)
		return c.SendStatus(http.StatusNoContent)
	})
	app.Delete("/logout", func(c fiber.Ctx) error {
		require.NoError(t, service.DelAdminAuth(c))
		return c.SendStatus(http.StatusNoContent)
	})
	app.Post("/approve-in-preview", func(c fiber.Ctx) error {
		operator, authErr := service.GetAdminAuth(c)
		require.NoError(t, authErr)
		_, bindErr := service.BindDelegatedRead(c.Context(), operator)
		require.ErrorIs(t, bindErr, ErrReadSessionDenied)
		return c.SendStatus(http.StatusForbidden)
	})
	issue := func(t *testing.T) *http.Cookie {
		t.Helper()
		login := performSessionRequest(t, app, http.MethodPost, "/login", nil)
		require.Equal(t, http.StatusNoContent, login.StatusCode)
		require.NoError(t, login.Body.Close())
		cookie := requireSessionCookie(t, login)
		approval := performSessionRequest(t, app, http.MethodPost, "/approve", cookie)
		require.Equal(t, http.StatusNoContent, approval.StatusCode)
		require.NoError(t, approval.Body.Close())
		return cookie
	}
	editSession := func(t *testing.T, cookie *http.Cookie, edit func(*session.Session)) {
		t.Helper()
		sess, getErr := store.GetByID(t.Context(), cookie.Value)
		require.NoError(t, getErr)
		defer sess.Release()
		edit(sess)
		require.NoError(t, sess.SaveWithContext(t.Context()))
	}
	t.Run("role preview and membership are read anew", func(t *testing.T) {
		cookie := issue(t)
		encoded, marshalErr := json.Marshal(map[string]ReadSessionBinding{"binding": pin})
		require.NoError(t, marshalErr)
		require.JSONEq(t, `{"binding":{}}`, string(encoded))
		require.NotContains(t, pin.storageKey, cookie.Value)
		key := auth.NewPermissionKey("product_events", auth.PermissionActionRead)
		ctx, current, resolveErr := service.ResolveDelegatedRead(t.Context(), pin)
		require.NoError(t, resolveErr)
		require.Equal(t, admin.ID, current.ID)
		require.False(t, auth.RolePreviewActive(ctx))
		editSession(t, cookie, func(s *session.Session) { s.Set(auth.RolePreviewSessionKey, int64(2)) })
		ctx, _, resolveErr = service.ResolveDelegatedRead(t.Context(), pin)
		require.NoError(t, resolveErr)
		require.True(t, auth.RolePreviewActive(ctx))
		require.False(t, auth.RolePreviewAllows(ctx, key))
		approve := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/approve-in-preview", nil)
		approve.AddCookie(cookie)
		denied, approvalErr := app.Test(approve)
		require.NoError(t, approvalErr)
		require.Equal(t, http.StatusForbidden, denied.StatusCode)
		require.NoError(t, denied.Body.Close())
		roles.permitted = true
		ctx, _, resolveErr = service.ResolveDelegatedRead(t.Context(), pin)
		require.NoError(t, resolveErr)
		require.True(t, auth.RolePreviewAllows(ctx, key))
		isOwner = false
		_, _, resolveErr = service.ResolveDelegatedRead(t.Context(), pin)
		require.ErrorIs(t, resolveErr, ErrReadSessionDenied)
		isOwner = true
		editSession(t, cookie, func(s *session.Session) { s.Delete(auth.RolePreviewSessionKey) })
		member.admin.ID++
		_, _, resolveErr = service.ResolveDelegatedRead(t.Context(), pin)
		require.ErrorIs(t, resolveErr, ErrReadSessionDenied)
		member.admin = models.Admin{}
		_, _, resolveErr = service.ResolveDelegatedRead(t.Context(), pin)
		require.ErrorIs(t, resolveErr, ErrReadSessionDenied)
		member.err = errors.New("membership store unavailable")
		_, _, resolveErr = service.ResolveDelegatedRead(t.Context(), pin)
		require.ErrorIs(t, resolveErr, member.err)
		member.err = nil
		member.admin = admin
	})
	t.Run("token refresh preserves approval and browser ID replacement requires fresh approval", func(t *testing.T) {
		cookie := issue(t)
		before, found, loadErr := journal.Load(t.Context(), cookie.Value)
		require.NoError(t, loadErr)
		require.True(t, found)
		now = now.Add(2 * time.Second)
		me := performSessionRequest(t, app, http.MethodGet, "/me", cookie)
		require.NoError(t, me.Body.Close())
		after, found, loadErr := journal.Load(t.Context(), cookie.Value)
		require.NoError(t, loadErr)
		require.True(t, found)
		require.NotEqual(t, before.Tokens.AccessToken, after.Tokens.AccessToken)
		_, _, resolveErr := service.ResolveDelegatedRead(t.Context(), pin)
		require.NoError(t, resolveErr)
		old := pin
		rotate := performSessionRequest(t, app, http.MethodPost, "/rotate-browser-id", cookie)
		require.NoError(t, rotate.Body.Close())
		newCookie := requireSessionCookie(t, rotate)
		require.NotEqual(t, cookie.Value, newCookie.Value)
		_, _, resolveErr = service.ResolveDelegatedRead(t.Context(), old)
		require.ErrorIs(t, resolveErr, ErrReadSessionEnded)
		approval := performSessionRequest(t, app, http.MethodPost, "/approve", newCookie)
		require.NoError(t, approval.Body.Close())
		require.Equal(t, old.loginID, pin.loginID)
		_, _, resolveErr = service.ResolveDelegatedRead(t.Context(), pin)
		require.NoError(t, resolveErr)
	})
	t.Run("logout closes only its browser grant", func(t *testing.T) {
		firstCookie := issue(t)
		first := pin
		issue(t)
		logout := performSessionRequest(t, app, http.MethodDelete, "/logout", firstCookie)
		require.NoError(t, logout.Body.Close())
		require.Equal(t, http.StatusNoContent, logout.StatusCode)
		_, _, firstErr := service.ResolveDelegatedRead(t.Context(), first)
		require.ErrorIs(t, firstErr, ErrReadSessionEnded)
		_, _, secondErr := service.ResolveDelegatedRead(t.Context(), pin)
		require.NoError(t, secondErr)
	})
	t.Run("canonical revocation precedes browser expiry", func(t *testing.T) {
		issue(t)
		require.NoError(t, fixture.Runtime.Logout(t.Context(), account.Subject.ID, pin.loginID))
		_, _, revokedErr := service.ResolveDelegatedRead(t.Context(), pin)
		require.ErrorIs(t, revokedErr, ErrReadSessionEnded)
	})
	t.Run("hard expiry and lost idle lease", func(t *testing.T) {
		cookie := issue(t)
		editSession(t, cookie, func(s *session.Session) {
			state, ok := s.Get(sessioncore.AuthAdminKey.String()).(*browserSession)
			require.True(t, ok)
			state.HardExpiresAt = time.Now().Add(-time.Second)
			s.Set(sessioncore.AuthAdminKey.String(), state)
		})
		_, _, expiredErr := service.ResolveDelegatedRead(t.Context(), pin)
		require.ErrorIs(t, expiredErr, ErrReadSessionEnded)
		cookie = issue(t)
		browserEntry, loadErr := store.Storage.GetWithContext(t.Context(), cookie.Value)
		require.NoError(t, loadErr)
		require.NotEmpty(t, browserEntry)
		require.NoError(t, store.Storage.SetWithContext(t.Context(), cookie.Value, browserEntry, time.Second))
		// Continuous delegated reads must allow the browser's idle lease to expire.
		require.EventuallyWithT(t, func(c *assert.CollectT) {
			_, _, idleErr := service.ResolveDelegatedRead(t.Context(), pin)
			require.ErrorIs(c, idleErr, ErrReadSessionEnded)
		}, 4*time.Second, 50*time.Millisecond)
	})
}

func TestDelegatedExternalReadFollowsOnlyTheApprovingLogin(t *testing.T) {
	f := newExternalFixture(t)
	require.NoError(t, WithDelegatedReadPolicy(func(context.Context, int64) bool { return true }, &delegatedPreview{})(f.service))
	f.service.identityLinkSessions = f.runtime.Store
	var pin ReadSessionBinding
	f.app.Post("/consent", func(c fiber.Ctx) error {
		admin, err := f.service.GetAdminAuth(c)
		require.NoError(t, err)
		_, err = f.service.BindDelegatedRead(c.Context(), admin)
		require.ErrorIs(t, err, ErrReadSessionEnded)
		pin, err = f.service.BindDelegatedReadWithExternal(c.Context(), admin)
		require.NoError(t, err)
		return c.SendStatus(http.StatusNoContent)
	})
	cookie := admittedExternal(t, f)
	approved := performSessionRequest(t, f.app, http.MethodPost, "/consent", cookie)
	require.NoError(t, approved.Body.Close())
	require.Equal(t, http.StatusNoContent, approved.StatusCode)
	resolve := func() error {
		_, _, err := f.service.ResolveDelegatedRead(t.Context(), pin)
		return err
	}
	require.NoError(t, resolve())
	f.resolver.replacement = true
	require.ErrorIs(t, resolve(), ErrReadSessionEnded)
	f.resolver.replacement = false

	// Publish a subsequent generation of this login whose short proof has expired.
	changeExternalRecoveryRecord(t, f, cookie, func(r *browserstate.Record) {
		r.External.Generation++
		r.External.Deadline = time.Now().Add(-time.Millisecond)
	})
	require.ErrorIs(t, resolve(), ErrExternalRecoveryRequired)
	f.app.Get("/renew-proof", func(c fiber.Ctx) error {
		snapshot, err := f.service.BeginExternalAdminRecovery(c)
		require.NoError(t, err)
		require.NoError(t, f.service.CompleteExternalAdminRecovery(c, snapshot, freshRecoveryBinding(f, snapshot.Binding())))
		return c.SendStatus(http.StatusNoContent)
	})
	renew := performSessionRequest(t, f.app, http.MethodGet, "/renew-proof", cookie)
	require.NoError(t, renew.Body.Close())
	require.Equal(t, http.StatusNoContent, renew.StatusCode)
	require.NoError(t, resolve())
	changeExternalRecoveryRecord(t, f, cookie, func(r *browserstate.Record) { r.External.LoginGeneration++ })
	require.ErrorIs(t, resolve(), ErrReadSessionEnded)
	changeExternalRecoveryRecord(t, f, cookie, func(r *browserstate.Record) { r.External.LoginGeneration-- })
	require.NoError(t, resolve())
	response, err := f.app.Test(requestWithCookie(http.MethodDelete, "/logout", cookie))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, response.Body.Close())
	require.ErrorIs(t, resolve(), ErrReadSessionEnded)
}
