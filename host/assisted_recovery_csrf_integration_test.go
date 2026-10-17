//go:build integration

package host //nolint:testpackage // Exercises the complete authenticated host middleware stack.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/rbac"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/internal/identity"
)

const recoveryCSRFOrigin = "https://admin.test"

type assembledRecoveryCommands struct{ calls atomic.Int32 }

func (s *assembledRecoveryCommands) Initiate(context.Context, goauth.AssistedRecoveryRequest) error {
	s.calls.Add(1)
	return nil
}

func (*assembledRecoveryCommands) Complete(context.Context, goauth.AssistedRecoveryCompletion) error {
	return nil
}

func newAssembledRecoveryFixture(t *testing.T, origin string) (*Runtime, *assembledRecoveryCommands, Config) {
	t.Helper()
	cfg, deps := assemblyFixture(t)
	parsed, err := url.Parse(origin)
	require.NoError(t, err)
	cfg.Admin.Domain = parsed.Hostname()
	cfg.Admin.BaseDomain = parsed.Hostname()
	cfg.CSRF.AppDomain = parsed.Hostname()
	cfg.Admin.DomainURL = origin
	cfg.Admin.BaseDomainURL = origin
	cfg.Server.AllowOrigins = []string{origin}
	cfg.CSRF.AllowedOrigins = []string{origin}
	cfg.Auth.NotificationDelivery = goauth.NotificationDeliveryDisabled
	commands := &assembledRecoveryCommands{}
	runtime, err := New(t.Context(), cfg, deps, AssistedRecoveryModule(AssistedRecoveryConfig{Commands: commands, Origin: origin}))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, runtime.Close()) })
	account, err := runtime.Auth.Runtime().ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
		Email: "recovery.form.operator@example.test", Password: assemblyPassword,
	})
	require.NoError(t, err)
	_, err = runtime.Auth.admins.ProvisionAccount(t.Context(), account, identity.NewUserID())
	require.NoError(t, err)
	roles, err := runtime.Auth.Runtime().RBAC(nil)
	require.NoError(t, err)
	role, err := roles.UpsertRole(t.Context(), rbac.Role{Slug: "super_admin", Name: "Owner", System: true})
	require.NoError(t, err)
	require.NoError(t, roles.ReplaceSubjectRoles(t.Context(), account.Subject.ID, []int64{role.ID}))
	return runtime, commands, cfg
}

func recoveryFixtureLogin(t *testing.T, runtime *Runtime) map[string]*http.Cookie {
	t.Helper()
	cookies := map[string]*http.Cookie{}
	get := httptest.NewRequestWithContext(t.Context(), http.MethodGet, recoveryCSRFOrigin+"/auth/login", nil)
	get.Header.Set("X-Inertia", "true")
	response, body := assemblyRequest(t, runtime, get, cookies)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, recoveryCSRFOrigin+"/auth/login",
		strings.NewReader(`{"email":"recovery.form.operator@example.test","password":"`+assemblyPassword+`"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", recoveryCSRFOrigin)
	request.Header.Set("X-CSRF-Token", cookies["csrf_token"].Value)
	response, body = assemblyRequest(t, runtime, request, cookies)
	require.Contains(t, []int{http.StatusFound, http.StatusSeeOther}, response.StatusCode, string(body))
	return cookies
}

func TestAssistedRecoveryAssembledNativeCSRF(t *testing.T) {
	runtime, commands, _ := newAssembledRecoveryFixture(t, recoveryCSRFOrigin)
	cookies := recoveryFixtureLogin(t, runtime)
	get := func(jar map[string]*http.Cookie) string {
		t.Helper()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, recoveryCSRFOrigin+AssistedRecoveryAdminPath, nil)
		request.Header.Set("X-Inertia", "true")
		response, body := assemblyRequest(t, runtime, request, jar)
		require.Equal(t, http.StatusOK, response.StatusCode, string(body))
		require.NotContains(t, string(body), "name=csrf")
		require.NotContains(t, response.Header.Get("Set-Cookie"), "goadmin_recovery")
		var page struct {
			Component string `json:"component"`
			Props     struct {
				NativeCSRF string `json:"nativeCSRF"`
			} `json:"props"`
		}
		require.NoError(t, json.Unmarshal(body, &page))
		require.Equal(t, "security/AssistedRecoveryPage", page.Component)
		token := page.Props.NativeCSRF
		require.NotEmpty(t, token)
		require.Equal(t, jar["csrf_token"].Value, token)
		return token
	}
	token := get(cookies) // Direct GET refreshes the post-login session-bound token.
	other := recoveryFixtureLogin(t, runtime)
	foreign := get(other)
	require.NotEqual(t, token, foreign)
	for _, scenario := range []string{"missing header", "missing cookie", "invalid signed token", "foreign session token", "foreign origin", "valid"} {
		t.Run(scenario, func(t *testing.T) {
			jar := map[string]*http.Cookie{}
			for name, cookie := range cookies {
				copyCookie := *cookie
				jar[name] = &copyCookie
			}
			header := token
			origin := recoveryCSRFOrigin
			switch scenario {
			case "missing header":
				header = ""
			case "missing cookie":
				delete(jar, "csrf_token")
			case "invalid signed token":
				header = "invalid"
				jar["csrf_token"].Value = header
			case "foreign session token":
				header = foreign
				jar["csrf_token"].Value = foreign
			case "foreign origin":
				origin = "https://foreign.example.test"
			}
			values := url.Values{
				"subject": {goauth.NewSubjectID().String()}, "email": {"restored@example.test"},
				"password": {assemblyPassword}, "code": {""}, "reason": {"Verified independently"},
				"evidence": {"Known support channel"}, "identity_verified": {"yes"},
			}
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, recoveryCSRFOrigin+AssistedRecoveryAdminPath,
				strings.NewReader(values.Encode()))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.Header.Set("Referer", recoveryCSRFOrigin+AssistedRecoveryAdminPath)
			request.Header.Set("Origin", origin)
			request.Header.Set("X-CSRF-Token", header)
			before := commands.calls.Load()
			response, body := assemblyRequest(t, runtime, request, jar)
			if scenario == "valid" {
				require.Equal(t, http.StatusAccepted, response.StatusCode, string(body))
				require.Equal(t, before+1, commands.calls.Load())
			} else {
				require.Equal(t, http.StatusFound, response.StatusCode, string(body))
				require.Equal(t, recoveryCSRFOrigin+AssistedRecoveryAdminPath, response.Header.Get("Location"))
				require.Equal(t, before, commands.calls.Load())
			}
		})
	}
}
