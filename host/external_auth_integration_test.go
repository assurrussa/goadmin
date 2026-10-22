//go:build integration

package host //nolint:testpackage // verifies public host assembly against disposable canonical storage.

import (
	"context"
	"errors"
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

const externalAssemblyFixtureName = "synthetic-external"

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

type externalAssemblyFeature struct {
	binding       *ExternalBinding
	lastLinkError *error
}

func (f externalAssemblyFeature) Descriptor() FeatureDescriptor {
	return FeatureDescriptor{Key: externalAssemblyFixtureName}
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
			routes.Get("/sso-fixture/link-status", func(c fiber.Ctx) error {
				linked, err := app.HasExternalAdminIdentity(c, f.binding.Issuer, f.binding.Subject)
				if err != nil {
					return err
				}
				if linked {
					return c.SendString("linked")
				}
				return c.SendString("unlinked")
			})
			routes.Get("/sso-fixture/eligibility", func(c fiber.Ctx) error {
				status, err := app.LocalIdentityLinkEligibility(c)
				if err != nil {
					return err
				}
				return c.SendString(string(status))
			})
			routes.Post("/sso-fixture/enroll", func(c fiber.Ctx) error {
				link, err := app.LinkExternalAdminIdentity(c, *f.binding)
				if f.lastLinkError != nil {
					*f.lastLinkError = err
				}
				if err != nil {
					return err
				}
				f.binding.LinkID = link.ID
				return c.SendStatus(http.StatusNoContent)
			})
			routes.Post("/sso-fixture/local-reauth", func(c fiber.Ctx) error {
				if err := app.LogoutLocalAdminAuth(c); err != nil {
					return err
				}
				return c.SendStatus(http.StatusNoContent)
			})
		}), nil
	}
}

func TestExternalAssemblyCanonicalLinkMembershipAndRegrant(t *testing.T) {
	cfg, deps := assemblyFixture(t)
	now := time.Now().UTC()
	authority := &externalAssemblyAuthority{proof: ExternalProof{IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Hour)}}
	binding := ExternalBinding{
		Issuer: "https://synthetic-issuer.invalid", ClientID: "synthetic-client", ProjectID: "synthetic-project",
		Subject: externalAssemblyFixtureName, AuthorityID: "synthetic-authority", Generation: 1, LoginGeneration: 1,
		Deadline: now.Add(5*time.Minute - time.Second), ProviderSessionID: "synthetic-provider-sid",
		ExternalAuthTime: now.Add(-time.Minute), AbsoluteUntil: now.Add(time.Hour), Proof: authority.proof,
	}
	deps.ExternalAuthority = authority
	runtime, err := New(t.Context(), cfg, deps, DefinedFeatureModule(
		ModuleDescriptor{Key: externalAssemblyFixtureName, RouteNamespaces: []string{"/sso-fixture"}},
		externalAssemblyFeature{binding: &binding},
	))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, runtime.Close()) })
	db := runtime.Auth.Runtime().Database()
	account, err := runtime.Auth.Runtime().ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
		Email: "synthetic-admin@example.invalid", Password: assemblyPassword,
	})
	require.NoError(t, err)
	admin, err := runtime.Auth.admins.ProvisionAccount(t.Context(), account, identity.NewUserID())
	require.NoError(t, err)
	binding.LocalSubject = account.Subject.ID
	cookies := map[string]*http.Cookie{}
	_, _ = assemblyGet(t, runtime, "/sso-fixture/callback", cookies)
	var count int
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM auth_identity_links").Scan(&count))
	require.Zero(t, count, "denied callback must not create a canonical link")
	response, body := assemblyGet(t, runtime, "/auth/login", cookies)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	loginBody := `{"email":"synthetic-admin@example.invalid","password":"` + assemblyPassword + `"}`
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		assemblyAdminOrigin+"/auth/login", strings.NewReader(loginBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", assemblyAdminOrigin)
	request.Header.Set("X-CSRF-Token", cookies["csrf_token"].Value)
	response, body = assemblyRequest(t, runtime, request, cookies)
	require.Contains(t, []int{http.StatusFound, http.StatusSeeOther}, response.StatusCode, string(body))
	response, body = assemblyGet(t, runtime, "/auth/profile", cookies)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	response, body = assemblyGet(t, runtime, "/sso-fixture/link-status", cookies)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	require.Equal(t, "unlinked", string(body))
	// A native form's body-only proof never reaches the registered logout route:
	// the assembled host's shared CSRF middleware requires the header as well.
	missingHeader := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		assemblyAdminOrigin+"/sso-fixture/local-reauth", strings.NewReader("csrf="+cookies["csrf_token"].Value))
	missingHeader.Header.Set("Origin", assemblyAdminOrigin)
	missingHeader.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, _ = assemblyRequest(t, runtime, missingHeader, cookies)
	require.NotEqual(t, http.StatusNoContent, response.StatusCode)
	response, body = assemblyGet(t, runtime, "/sso-fixture/link-status", cookies)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	require.Equal(t, "unlinked", string(body), "body-only CSRF unexpectedly logged out the local session")
	enrollment := httptest.NewRequestWithContext(t.Context(), http.MethodPost, assemblyAdminOrigin+"/sso-fixture/enroll", nil)
	enrollment.Header.Set("Origin", assemblyAdminOrigin)
	enrollment.Header.Set("X-CSRF-Token", cookies["csrf_token"].Value)
	response, body = assemblyRequest(t, runtime, enrollment, cookies)
	require.Equal(t, http.StatusNoContent, response.StatusCode, string(body))
	require.NotEmpty(t, binding.LinkID)
	response, body = assemblyGet(t, runtime, "/sso-fixture/link-status", cookies)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	require.Equal(t, "linked", string(body))
	links, err := postgres.NewStore(db)
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
	localReauth := httptest.NewRequestWithContext(t.Context(), http.MethodPost, assemblyAdminOrigin+"/sso-fixture/local-reauth", nil)
	localReauth.Header.Set("Origin", assemblyAdminOrigin)
	localReauth.Header.Set("X-CSRF-Token", cookies["csrf_token"].Value)
	response, _ = assemblyRequest(t, runtime, localReauth, cookies)
	require.NotEqual(t, http.StatusNoContent, response.StatusCode, "external session accepted local-only logout")
	require.False(t, authority.invalidated, "local-only reauth revoked provider")
	response, body = assemblyGet(t, runtime, "/auth/profile", cookies)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	// The actual canonical row gets a new link instance ID, equivalent to unlink
	// plus confirmed regrant to the SAME local subject. Old native sessions deny.
	_, err = db.ExecContext(t.Context(), "UPDATE auth_identity_links SET id=$1 WHERE id=$2", uuid.NewString(), link.ID)
	require.NoError(t, err)
	response, _ = assemblyGet(t, runtime, "/auth/profile", cookies)
	require.Contains(t, []int{http.StatusFound, http.StatusSeeOther, http.StatusUnauthorized}, response.StatusCode)
	_, err = db.ExecContext(t.Context(),
		"UPDATE auth_identity_links SET id=$1 WHERE issuer=$2 AND external_subject=$3", link.ID, binding.Issuer, binding.Subject)
	require.NoError(t, err)
	response, _ = assemblyGet(t, runtime, "/auth/profile", cookies)
	require.Contains(t, []int{http.StatusFound, http.StatusSeeOther, http.StatusUnauthorized}, response.StatusCode,
		"restoring a link must not resurrect cleared native authority")
	current, err := runtime.Auth.admins.GetByID(t.Context(), admin.ID)
	require.NoError(t, err)
	require.Equal(t, admin.GetRoles(), current.GetRoles())
	require.Equal(t, admin.GetPermissions(), current.GetPermissions())
	// A real canonical member removal must reject a new external admission.
	_, err = db.ExecContext(t.Context(), "DELETE FROM administrations WHERE id=$1", admin.ID)
	require.NoError(t, err)
	var before, after int
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM auth_sessions").Scan(&before))
	response, _ = assemblyGet(t, runtime, "/sso-fixture/callback", cookies)
	require.NotEqual(t, http.StatusNoContent, response.StatusCode)
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM auth_sessions").Scan(&after))
	require.Equal(t, before, after, "nonmember callback must not issue a native session")
}

func TestExternalAssemblyExpiredCanonicalLinkFreshness(t *testing.T) {
	cfg, deps := assemblyFixture(t)
	cfg.Auth.IdentityLinkAuthMaxAge = 2 * time.Minute
	now := time.Now().UTC()
	authority := &externalAssemblyAuthority{proof: ExternalProof{IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Hour)}}
	binding := ExternalBinding{
		Issuer: "https://synthetic-issuer.invalid", ClientID: "synthetic-client", ProjectID: "synthetic-project",
		Subject: externalAssemblyFixtureName, AuthorityID: "synthetic-authority", Generation: 1, LoginGeneration: 1,
		Deadline: now.Add(5*time.Minute - time.Second), ProviderSessionID: "synthetic-provider-sid",
		ExternalAuthTime: now.Add(-time.Minute), AbsoluteUntil: now.Add(time.Hour), Proof: authority.proof,
	}
	deps.ExternalAuthority = authority
	var linkErr error
	runtime, err := New(t.Context(), cfg, deps, DefinedFeatureModule(
		ModuleDescriptor{Key: externalAssemblyFixtureName, RouteNamespaces: []string{"/sso-fixture"}},
		externalAssemblyFeature{binding: &binding, lastLinkError: &linkErr},
	))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, runtime.Close()) })
	account, err := runtime.Auth.Runtime().ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
		Email: "stale-admin@example.invalid", Password: assemblyPassword,
	})
	require.NoError(t, err)
	_, err = runtime.Auth.admins.ProvisionAccount(t.Context(), account, identity.NewUserID())
	require.NoError(t, err)
	binding.LocalSubject = account.Subject.ID
	cookies := map[string]*http.Cookie{}
	response, body := assemblyGet(t, runtime, "/auth/login", cookies)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		assemblyAdminOrigin+"/auth/login", strings.NewReader(`{"email":"stale-admin@example.invalid","password":"`+assemblyPassword+`"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", assemblyAdminOrigin)
	request.Header.Set("X-CSRF-Token", cookies["csrf_token"].Value)
	response, body = assemblyRequest(t, runtime, request, cookies)
	require.Contains(t, []int{http.StatusFound, http.StatusSeeOther}, response.StatusCode, string(body))
	// The UI and GoAuth use one configured window, based on canonical creation
	// rather than a refreshed access token.
	_, err = runtime.Auth.Runtime().Database().ExecContext(t.Context(),
		"UPDATE auth_sessions SET created_at=now()-interval '90 seconds' WHERE subject_id=$1 AND realm=$2",
		account.Subject.ID, goauth.RealmAdmin)
	require.NoError(t, err)
	response, body = assemblyGet(t, runtime, "/sso-fixture/eligibility", cookies)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	require.Equal(t, string(IdentityLinkFresh), string(body))
	// Simulate an old canonical session while leaving the native browser session and external proof valid.
	result, err := runtime.Auth.Runtime().Database().ExecContext(t.Context(),
		"UPDATE auth_sessions SET created_at=now()-interval '3 minutes' WHERE subject_id=$1 AND realm=$2",
		account.Subject.ID, goauth.RealmAdmin)
	require.NoError(t, err)
	updated, err := result.RowsAffected()
	require.NoError(t, err)
	require.EqualValues(t, 1, updated)
	response, body = assemblyGet(t, runtime, "/auth/profile", cookies)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	response, body = assemblyGet(t, runtime, "/sso-fixture/eligibility", cookies)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	require.Equal(t, string(IdentityLinkStale), string(body))
	enrollment := httptest.NewRequestWithContext(t.Context(), http.MethodPost, assemblyAdminOrigin+"/sso-fixture/enroll", nil)
	enrollment.Header.Set("Origin", assemblyAdminOrigin)
	enrollment.Header.Set("X-CSRF-Token", cookies["csrf_token"].Value)
	response, _ = assemblyRequest(t, runtime, enrollment, cookies)
	require.NotEqual(t, http.StatusNoContent, response.StatusCode)
	require.True(t, errors.Is(linkErr, goauth.ErrExplicitIdentityLink), "expected stale canonical session error, got %v", linkErr)
	var links int
	require.NoError(t, runtime.Auth.Runtime().Database().QueryRowContext(t.Context(),
		"SELECT count(*) FROM auth_identity_links").Scan(&links))
	require.Zero(t, links)
}
