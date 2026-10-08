//go:build integration

package host //nolint:testpackage // verifies public host assembly against disposable canonical storage.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/postgres"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/internal/identity"
)

type externalAssemblyAuthority struct {
	proof       ExternalProof
	invalidated bool
}

func (a *externalAssemblyAuthority) Validate(context.Context, ExternalBinding) (ExternalProof, error) {
	if a.invalidated {
		return ExternalProof{}, goauth.ErrSessionRevoked
	}
	return a.proof, nil
}

func (a *externalAssemblyAuthority) Logout(context.Context, ExternalBinding) error {
	a.invalidated = true
	return nil
}

func (a *externalAssemblyAuthority) Detach(context.Context, ExternalBinding) error { return nil }

type externalAssemblyFeature struct{ binding *ExternalBinding }

func (f externalAssemblyFeature) Descriptor() FeatureDescriptor {
	return FeatureDescriptor{Key: "synthetic-external"}
}

func (f externalAssemblyFeature) Build(Menu) ExtensionBuilder {
	return func(app *App) (*Extension, error) {
		return NewExtension().WithPublicRegister(func(routes *fiber.App) {
			routes.Get("/sso-fixture/callback", func(c fiber.Ctx) error {
				_, err := app.LoginExternalAdmin(c, *f.binding)
				if err != nil {
					return err
				}
				return c.SendStatus(http.StatusNoContent)
			})
		}).WithRegister(func(routes *fiber.App) {
			routes.Post("/sso-fixture/enroll", func(c fiber.Ctx) error {
				link, err := app.LinkExternalAdminIdentity(c, *f.binding)
				if err != nil {
					return err
				}
				f.binding.LinkID = link.ID
				return c.SendStatus(http.StatusNoContent)
			})
		}), nil
	}
}

func TestExternalAssemblyCanonicalLinkMembershipAndRegrant(t *testing.T) {
	cfg, deps := assemblyFixture(t)
	now := time.Now().UTC()
	authority := &externalAssemblyAuthority{proof: ExternalProof{IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Hour)}}
	binding := ExternalBinding{Issuer: "https://synthetic-issuer.invalid", Subject: "synthetic-external", AuthorityID: "synthetic-authority", Generation: 1, LoginGeneration: 1, Deadline: now.Add(5*time.Minute - time.Second), ProviderSessionID: "synthetic-provider-sid", ExternalAuthTime: now.Add(-time.Minute), AbsoluteUntil: now.Add(time.Hour), Proof: authority.proof}
	deps.ExternalAuthority = authority
	runtime, err := New(t.Context(), cfg, deps, DefinedFeatureModule(ModuleDescriptor{Key: "synthetic-external", RouteNamespaces: []string{"/sso-fixture"}}, externalAssemblyFeature{&binding}))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, runtime.Close()) })
	account, err := runtime.Auth.Runtime().ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{Email: "synthetic-admin@example.invalid", Password: assemblyPassword})
	require.NoError(t, err)
	admin, err := runtime.Auth.admins.ProvisionAccount(t.Context(), account, identity.NewUserID())
	require.NoError(t, err)
	binding.LocalSubject = account.Subject.ID
	cookies := map[string]*http.Cookie{}
	_, body := assemblyGet(t, runtime, "/sso-fixture/callback", cookies)
	var count int
	require.NoError(t, runtime.Auth.Runtime().Database().QueryRowContext(t.Context(), "SELECT count(*) FROM auth_identity_links").Scan(&count))
	require.Zero(t, count, "denied callback must not create a canonical link")
	response, body := assemblyGet(t, runtime, "/auth/login", cookies)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, assemblyAdminOrigin+"/auth/login", strings.NewReader(`{"email":"synthetic-admin@example.invalid","password":"`+assemblyPassword+`"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", assemblyAdminOrigin)
	request.Header.Set("X-CSRF-Token", cookies["csrf_token"].Value)
	response, body = assemblyRequest(t, runtime, request, cookies)
	require.Contains(t, []int{http.StatusFound, http.StatusSeeOther}, response.StatusCode, string(body))
	response, body = assemblyGet(t, runtime, "/auth/profile", cookies)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	enrollment := httptest.NewRequestWithContext(t.Context(), http.MethodPost, assemblyAdminOrigin+"/sso-fixture/enroll", nil)
	enrollment.Header.Set("Origin", assemblyAdminOrigin)
	enrollment.Header.Set("X-CSRF-Token", cookies["csrf_token"].Value)
	response, body = assemblyRequest(t, runtime, enrollment, cookies)
	require.Equal(t, http.StatusNoContent, response.StatusCode, string(body))
	require.NotEmpty(t, binding.LinkID)
	links, err := postgres.NewStore(runtime.Auth.Runtime().Database())
	require.NoError(t, err)
	linked, link, err := links.ResolveIdentityLink(t.Context(), binding.Issuer, binding.Subject)
	require.NoError(t, err)
	require.Equal(t, account.Subject.ID, linked.Subject.ID)
	require.Equal(t, binding.LinkID, link.ID)
	oldCookie := *cookies[cfg.Admin.SessionName]
	response, body = assemblyGet(t, runtime, "/sso-fixture/callback", cookies)
	require.Equal(t, http.StatusNoContent, response.StatusCode, string(body))
	require.NotEqual(t, oldCookie.Value, cookies[cfg.Admin.SessionName].Value)
	response, body = assemblyGet(t, runtime, "/auth/profile", cookies)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	// The actual canonical row gets a new link instance ID, equivalent to unlink
	// plus confirmed regrant to the SAME local subject. Old native sessions deny.
	_, err = runtime.Auth.Runtime().Database().ExecContext(t.Context(), "UPDATE auth_identity_links SET id=$1 WHERE id=$2", uuid.NewString(), link.ID)
	require.NoError(t, err)
	response, _ = assemblyGet(t, runtime, "/auth/profile", cookies)
	require.Contains(t, []int{http.StatusFound, http.StatusSeeOther, http.StatusUnauthorized}, response.StatusCode)
	_, err = runtime.Auth.Runtime().Database().ExecContext(t.Context(), "UPDATE auth_identity_links SET id=$1 WHERE issuer=$2 AND external_subject=$3", link.ID, binding.Issuer, binding.Subject)
	require.NoError(t, err)
	response, _ = assemblyGet(t, runtime, "/auth/profile", cookies)
	require.Contains(t, []int{http.StatusFound, http.StatusSeeOther, http.StatusUnauthorized}, response.StatusCode, "restoring a link must not resurrect cleared native authority")
	current, err := runtime.Auth.admins.GetByID(t.Context(), admin.ID)
	require.NoError(t, err)
	require.Equal(t, admin.GetRoles(), current.GetRoles())
	require.Equal(t, admin.GetPermissions(), current.GetPermissions())
	// A real canonical member removal must reject a new external admission.
	_, err = runtime.Auth.Runtime().Database().ExecContext(t.Context(), "DELETE FROM administrations WHERE id=$1", admin.ID)
	require.NoError(t, err)
	var before, after int
	require.NoError(t, runtime.Auth.Runtime().Database().QueryRowContext(t.Context(), "SELECT count(*) FROM auth_sessions").Scan(&before))
	response, _ = assemblyGet(t, runtime, "/sso-fixture/callback", cookies)
	require.NotEqual(t, http.StatusNoContent, response.StatusCode)
	require.NoError(t, runtime.Auth.Runtime().Database().QueryRowContext(t.Context(), "SELECT count(*) FROM auth_sessions").Scan(&after))
	require.Equal(t, before, after, "nonmember callback must not issue a native session")
}
