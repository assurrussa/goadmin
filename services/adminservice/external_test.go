//nolint:testpackage,lll // Exercises private canonical journal state with explicit synthetic fixture cases.
package adminservice

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/testkit"
	"github.com/gofiber/fiber/v3"
	fibersession "github.com/gofiber/fiber/v3/middleware/session"
	"github.com/stretchr/testify/require"

	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/auth/browserstate"
	"github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
)

type externalAuthorityFake struct {
	proof                     ExternalProof
	err                       error
	invalidated               int
	detached                  int
	lastDetachGeneration      uint64
	lastLogoutGeneration      uint64
	lastLogoutLoginGeneration uint64
}

func (a *externalAuthorityFake) Validate(_ context.Context, _ ExternalBinding) (ExternalProof, error) {
	if a.invalidated > 0 {
		return ExternalProof{}, goauth.ErrSessionRevoked
	}
	return a.proof, a.err
}

func (a *externalAuthorityFake) Logout(_ context.Context, b ExternalBinding) error {
	a.invalidated++
	a.lastLogoutGeneration = b.Generation
	a.lastLogoutLoginGeneration = b.LoginGeneration
	return a.err
}

func (a *externalAuthorityFake) Detach(_ context.Context, b ExternalBinding) error {
	a.detached++
	a.lastDetachGeneration = b.Generation
	return nil
}

type linkResolverFake struct {
	IdentityLinkResolver
	missing     bool
	replacement bool
	err         error
}

func (r *linkResolverFake) ResolveIdentityLink(ctx context.Context, issuer, subject string) (goauth.Account, goauth.IdentityLink, error) {
	if r.err != nil {
		return goauth.Account{}, goauth.IdentityLink{}, r.err
	}
	if r.missing {
		return goauth.Account{}, goauth.IdentityLink{}, goauth.ErrIdentityLinkNotFound
	}
	account, link, err := r.IdentityLinkResolver.ResolveIdentityLink(ctx, issuer, subject)
	if r.replacement {
		link.ID = "synthetic-replacement-link"
	}
	return account, link, err
}

type externalFixture struct {
	service   *Service
	app       *fiber.App
	runtime   *testkit.Fixture
	binding   *ExternalBinding
	authority *externalAuthorityFake
	resolver  *linkResolverFake
}

func newExternalFixture(t *testing.T, options ...testkit.RuntimeOption) externalFixture {
	t.Helper()
	f, err := testkit.NewRuntime(options...)
	require.NoError(t, err)
	account, err := f.Runtime.ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{Email: "synthetic-admin@example.invalid", Password: testAdminPassword})
	require.NoError(t, err)
	admin := models.Admin{ID: 91, SubjectID: account.Subject.ID, UUID: identity.NewUserID(), Data: &models.AdminData{Permissions: map[string][]string{"synthetic-local": {"read"}}}}
	now := time.Now().UTC()
	authority := &externalAuthorityFake{proof: ExternalProof{now.Add(-time.Second), now.Add(time.Hour)}}
	resolver := &linkResolverFake{IdentityLinkResolver: f.Store}
	svc := NewService(fibersession.NewStore(), roleServiceStub{}, WithRuntime(f.Runtime, adminRepositoryStub{admin: admin}), WithBrowserState(browserstate.NewMemory()), WithExternalAuthority(authority, resolver))
	binding := ExternalBinding{Issuer: "https://synthetic-issuer.invalid", ClientID: "synthetic-client", ProjectID: "synthetic-project", Subject: "synthetic-external", AuthorityID: "synthetic-provider-session", Generation: 1, LoginGeneration: 1, Deadline: now.Add(5*time.Minute - time.Second), ProviderSessionID: "synthetic-sid", ExternalAuthTime: now.Add(-time.Minute), LocalSubject: account.Subject.ID, AbsoluteUntil: now.Add(time.Hour), Proof: authority.proof}
	app := fiber.New()
	app.Post("/local", func(c fiber.Ctx) error {
		_, _, err := svc.LoginAdmin(c, account.PrimaryEmail.DisplayValue, testAdminPassword, false)
		if err != nil {
			return err
		}
		return c.SendStatus(http.StatusNoContent)
	})
	app.Post("/enroll", func(c fiber.Ctx) error {
		link, err := svc.LinkExternalAdminIdentity(c, binding)
		if err != nil {
			return err
		}
		binding.LinkID = link.ID
		return c.SendStatus(http.StatusNoContent)
	})
	app.Post("/external", func(c fiber.Ctx) error {
		_, err := svc.LoginExternalAdmin(c, binding)
		if err != nil {
			return err
		}
		return c.SendStatus(http.StatusNoContent)
	})
	app.Get("/auth", func(c fiber.Ctx) error {
		a, err := svc.GetAdminAuth(c)
		if err != nil {
			return err
		}
		if a == nil {
			return fiber.ErrUnauthorized
		}
		require.Equal(t, []string{"read"}, a.Permissions["synthetic-local"])
		require.False(t, a.ExternalValidUntil.IsZero())
		return c.SendStatus(http.StatusNoContent)
	})
	app.Delete("/logout", func(c fiber.Ctx) error { return svc.DelAdminAuth(c) })
	return externalFixture{svc, app, f, &binding, authority, resolver}
}

func TestExternalEnrollmentAdmissionUsesCanonicalLinkAndRotatesJournal(t *testing.T) {
	f := newExternalFixture(t)
	response, err := f.app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/external", nil))
	require.NoError(t, err)
	require.GreaterOrEqual(t, response.StatusCode, 400)
	_ = response.Body.Close()
	// No canonical link was created by denied external login.
	_, _, err = f.runtime.Store.ResolveIdentityLink(t.Context(), f.binding.Issuer, f.binding.Subject)
	require.ErrorIs(t, err, goauth.ErrIdentityLinkNotFound)
	local := performSessionRequest(t, f.app, http.MethodPost, "/local", nil)
	cookie := requireSessionCookie(t, local)
	_ = local.Body.Close()
	enrolled := performSessionRequest(t, f.app, http.MethodPost, "/enroll", cookie)
	_ = enrolled.Body.Close()
	account, link, err := f.runtime.Store.ResolveIdentityLink(t.Context(), f.binding.Issuer, f.binding.Subject)
	require.NoError(t, err)
	require.Equal(t, f.binding.LocalSubject, account.Subject.ID)
	require.NotEmpty(t, link.ID)
	admitted := performSessionRequest(t, f.app, http.MethodPost, "/external", cookie)
	fresh := requireSessionCookie(t, admitted)
	require.NotEqual(t, cookie.Value, fresh.Value)
	_ = admitted.Body.Close()
	authenticated := performSessionRequest(t, f.app, http.MethodGet, "/auth", fresh)
	_ = authenticated.Body.Close()
	old, err := f.app.Test(requestWithCookie(http.MethodGet, "/auth", cookie))
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, old.StatusCode)
	_ = old.Body.Close()
	require.Equal(t, 0, f.authority.invalidated)
}

func requestWithCookie(method, path string, cookie *http.Cookie) *http.Request {
	r := httptest.NewRequestWithContext(context.Background(), method, path, nil)
	r.AddCookie(cookie)
	return r
}

func admittedExternal(t *testing.T, f externalFixture) *http.Cookie {
	t.Helper()
	local := performSessionRequest(t, f.app, http.MethodPost, "/local", nil)
	cookie := requireSessionCookie(t, local)
	_ = local.Body.Close()
	enrolled := performSessionRequest(t, f.app, http.MethodPost, "/enroll", cookie)
	_ = enrolled.Body.Close()
	response := performSessionRequest(t, f.app, http.MethodPost, "/external", cookie)
	fresh := requireSessionCookie(t, response)
	_ = response.Body.Close()
	return fresh
}

func TestExternalEveryRequestFailsClosedWithoutDowngrade(t *testing.T) {
	for _, name := range []string{"missing-link", "regrant", "resolver-outage", "authority-outage", "authority-disabled", "expired", "receipt-slide", "absolute-end"} {
		t.Run(name, func(t *testing.T) {
			f := newExternalFixture(t)
			cookie := admittedExternal(t, f)
			switch name {
			case "missing-link":
				f.resolver.missing = true
			case "regrant":
				f.resolver.replacement = true
			case "resolver-outage":
				f.resolver.err = errors.New("synthetic link store failure")
			case "authority-outage":
				f.authority.err = errors.New("synthetic provider store failure")
			case "authority-disabled":
				f.service.externalAuthority = nil
			case "expired":
				f.authority.proof = ExternalProof{time.Now().Add(-6 * time.Minute), time.Now().Add(time.Hour)}
			case "receipt-slide":
				f.authority.proof.ExpiresAt = f.authority.proof.ExpiresAt.Add(time.Hour)
			case "absolute-end":
				f.authority.proof = ExternalProof{time.Now().Add(time.Hour), time.Now().Add(2 * time.Hour)}
			}
			response, err := f.app.Test(requestWithCookie(http.MethodGet, "/auth", cookie))
			require.NoError(t, err)
			require.GreaterOrEqual(t, response.StatusCode, 400)
			_ = response.Body.Close()
			f.resolver.missing = false
			f.resolver.replacement = false
			f.resolver.err = nil
			f.authority.err = nil
			retry, err := f.app.Test(requestWithCookie(http.MethodGet, "/auth", cookie))
			require.NoError(t, err)
			require.Equal(t, http.StatusUnauthorized, retry.StatusCode)
			_ = retry.Body.Close()
		})
	}
}

func TestExternalLogoutKeepsLocalInvalidationOnProviderFailure(t *testing.T) {
	f := newExternalFixture(t)
	cookie := admittedExternal(t, f)
	f.authority.err = errors.New("synthetic revoke outage")
	response, err := f.app.Test(requestWithCookie(http.MethodDelete, "/logout", cookie))
	require.NoError(t, err)
	require.GreaterOrEqual(t, response.StatusCode, 400)
	_ = response.Body.Close()
	require.Equal(t, 1, f.authority.invalidated)
	f.authority.err = nil
	response, err = f.app.Test(requestWithCookie(http.MethodGet, "/auth", cookie))
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
	_ = response.Body.Close()
}

func TestExternalEnrollmentRequiresFreshCanonicalLocalSession(t *testing.T) {
	now := time.Now().UTC()
	f := newExternalFixture(t, func(c *goauth.Config) {
		c.Now = func() time.Time { return now }
		c.IdentityLinkAuthMaxAge = time.Minute
	})
	absent, err := f.app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/enroll", nil))
	require.NoError(t, err)
	require.GreaterOrEqual(t, absent.StatusCode, 400)
	_ = absent.Body.Close()
	local := performSessionRequest(t, f.app, http.MethodPost, "/local", nil)
	cookie := requireSessionCookie(t, local)
	_ = local.Body.Close()
	now = now.Add(2 * time.Minute)
	expired, err := f.app.Test(requestWithCookie(http.MethodPost, "/enroll", cookie))
	require.NoError(t, err)
	require.GreaterOrEqual(t, expired.StatusCode, 400)
	_ = expired.Body.Close()
	_, _, err = f.runtime.Store.ResolveIdentityLink(t.Context(), f.binding.Issuer, f.binding.Subject)
	require.ErrorIs(t, err, goauth.ErrIdentityLinkNotFound)
}

type delayedExternalRoles struct {
	roleService
	delay time.Duration
}

func (r delayedExternalRoles) GetRolesAdmin(ctx context.Context, id int64) ([]integrationroles.Role, error) {
	time.Sleep(r.delay)
	return r.roleService.GetRolesAdmin(ctx, id)
}

func TestExternalProofCannotExpireDuringNativeRefreshOrACLLoad(t *testing.T) {
	f := newExternalFixture(t)
	f.binding.Deadline = time.Now().Add(2 * time.Second)
	cookie := admittedExternal(t, f)
	f.service.roleService = delayedExternalRoles{roleService: roleServiceStub{}, delay: 2100 * time.Millisecond}
	response, err := f.app.Test(requestWithCookie(http.MethodGet, "/auth", cookie), fiber.TestConfig{Timeout: 4 * time.Second})
	require.NoError(t, err)
	require.GreaterOrEqual(t, response.StatusCode, 400)
	_ = response.Body.Close()
}

func TestExternalReadmissionDetachesWithoutProviderLogout(t *testing.T) {
	for _, generation := range []uint64{1, 2} {
		t.Run(fmt.Sprintf("generation-%d", generation), func(t *testing.T) {
			f := newExternalFixture(t)
			cookie := admittedExternal(t, f)
			f.binding.Generation = generation
			admitted := performSessionRequest(t, f.app, http.MethodPost, "/external", cookie)
			fresh := requireSessionCookie(t, admitted)
			_ = admitted.Body.Close()
			require.NotEqual(t, cookie.Value, fresh.Value)
			require.Equal(t, 1, f.authority.detached)
			require.Equal(t, uint64(1), f.authority.lastDetachGeneration)
			require.Equal(t, 0, f.authority.invalidated)
			authenticated := performSessionRequest(t, f.app, http.MethodGet, "/auth", fresh)
			_ = authenticated.Body.Close()
		})
	}
}

func TestExplicitExternalLogoutForwardsSupersededSnapshot(t *testing.T) {
	f := newExternalFixture(t)
	cookie := admittedExternal(t, f)
	// Provider verification can have advanced while native admission is still N.
	// Logout must be dispatched as user intent, not ignored by generation mismatch.
	f.binding.Generation = 2
	response, err := f.app.Test(requestWithCookie(http.MethodDelete, "/logout", cookie))
	require.NoError(t, err)
	require.Less(t, response.StatusCode, 400)
	_ = response.Body.Close()
	require.Equal(t, 1, f.authority.invalidated)
	require.Equal(t, uint64(1), f.authority.lastLogoutGeneration)
	require.Equal(t, uint64(1), f.authority.lastLogoutLoginGeneration)
	require.Equal(t, 0, f.authority.detached)
}
