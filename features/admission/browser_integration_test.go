//go:build integration && browser

package admission_test

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/postgres"
	"github.com/assurrussa/goauth/rbac"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/features/access"
	admission "github.com/assurrussa/goadmin/features/admission"
	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/tests"
)

// This authority exists only in an explicitly tagged test binary. Browser input
// selects a named synthetic scenario, never issuer/subject/claims or credentials.
type browserAdmissionAuthority struct {
	mu sync.Mutex
	identities map[string]admission.VerifiedIdentity
	bindings map[string]host.ExternalBinding
	states map[string]browserAdmissionState
	owners map[string]string
	scope admission.Scope
	observations []browserAdmissionObservation
}
type browserAdmissionState struct { browser, reference, invitation string }
type browserAdmissionObservation struct {
	Method string `json:"method"`
	Origin string `json:"origin"`
	Referer string `json:"referer"`
	HasCSRF bool `json:"hasCSRF"`
	HasSession bool `json:"hasSession"`
}

func (a *browserAdmissionAuthority) Verify(_ context.Context, ref string) (admission.VerifiedIdentity, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	i, ok := a.identities[ref]
	if !ok || !time.Now().Before(i.ExpiresAt) { return admission.VerifiedIdentity{}, goauth.ErrSessionRevoked }
	return i, nil
}
func (a *browserAdmissionAuthority) Revalidate(ctx context.Context, ref string, i admission.VerifiedIdentity) error {
	current, err := a.Verify(ctx, ref)
	if err != nil { return err }
	if current != i { return goauth.ErrSessionRevoked }
	return nil
}
func (a *browserAdmissionAuthority) Validate(_ context.Context, b host.ExternalBinding) (host.ExternalProof, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	want, ok := a.bindings[b.AuthorityID]
	if !ok || want != b { return host.ExternalProof{}, goauth.ErrSessionRevoked }
	return want.Proof, nil
}
func (*browserAdmissionAuthority) Detach(context.Context, host.ExternalBinding) error { return nil }
func (a *browserAdmissionAuthority) Logout(_ context.Context, b host.ExternalBinding) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.bindings, b.AuthorityID)
	return nil
}

type browserAdmissionFeature struct {
	authority *browserAdmissionAuthority
	component *admission.Component
	auth *host.AuthAdapter
}
func (f browserAdmissionFeature) Descriptor() host.FeatureDescriptor { return host.FeatureDescriptor{Key: "admission-browser-fixture"} }
func (f browserAdmissionFeature) Build(host.Menu) host.ExtensionBuilder {
	return func(app *host.App) (*host.Extension, error) {
		return host.NewExtension().WithMiddlewares(func(c fiber.Ctx) error {
			if c.Method() == fiber.MethodPost && c.Path() == "/admissions/approve" {
				f.authority.mu.Lock()
				f.authority.observations = append(f.authority.observations, browserAdmissionObservation{
					Method: c.Method(), Origin: c.Get("Origin"), Referer: c.Get("Referer"),
					HasCSRF: c.Get("X-CSRF-Token") != "", HasSession: c.Cookies("admission_browser_session") != "",
				})
				f.authority.mu.Unlock()
			}
			return c.Next()
		}).WithPublicRegister(func(routes *fiber.App) {
			routes.Get("/admission-browser/start", func(c fiber.Ctx) error {
				scenario := c.Query("scenario", "alice")
				if scenario != "alice" && scenario != "wrong" && scenario != "no-project" { return fiber.ErrBadRequest }
				browser := c.Cookies("admission_fixture_browser")
				if browser == "" {
					browser = uuid.NewString()
					c.Cookie(&fiber.Cookie{Name: "admission_fixture_browser", Value: browser, Secure: true, HTTPOnly: true, SameSite: "Lax", Path: "/"})
				}
				ref, state := uuid.NewString(), uuid.NewString()
				i := admission.VerifiedIdentity{Scope: f.authority.scope, Subject: scenario, Email: scenario+"@example.invalid", EmailVerified: true, ExpiresAt: time.Now().Add(time.Hour)}
				// A missing central project grant fails verification before enrollment.
				if scenario == "no-project" { i.Subject = "invited"; i.Email = "invited@example.invalid"; i.Scope.Project = "ungranted-project" }
				f.authority.mu.Lock()
				f.authority.identities[ref] = i
				f.authority.owners[ref] = browser
				f.authority.states[state] = browserAdmissionState{browser: browser, reference: ref, invitation: c.Query("invitation")}
				f.authority.mu.Unlock()
				return c.Redirect().To("/admission-browser/callback?state="+state)
			})
			routes.Get("/admission-browser/callback", func(c fiber.Ctx) error {
				f.authority.mu.Lock()
				state, ok := f.authority.states[c.Query("state")]
				if ok && state.browser == c.Cookies("admission_fixture_browser") { delete(f.authority.states, c.Query("state")) } else { ok = false }
				f.authority.mu.Unlock()
				if !ok { return fiber.ErrForbidden }
				i, err := f.authority.Verify(c, state.reference)
				if err != nil { return fiber.ErrForbidden }
				result, err := f.component.Complete(c, state.reference, state.invitation)
				if err != nil { return fiber.ErrForbidden }
				c.Cookie(&fiber.Cookie{Name: "admission_fixture_reference", Value: state.reference, Secure: true, HTTPOnly: true, SameSite: "Lax", Path: "/"})
				if result.Status != "active" { return c.Redirect().To("/auth/admission") }
				store, err := postgres.NewStore(f.auth.Runtime().Database())
				if err != nil { return err }
				account, link, err := store.ResolveIdentityLink(c, i.Scope.Issuer, i.Subject)
				if err != nil || account.Subject.ID.String() != result.LocalSubject { return fiber.ErrForbidden }
				now := time.Now().UTC()
				b := host.ExternalBinding{
					Issuer: i.Scope.Issuer, Subject: i.Subject, ClientID: i.Scope.Client, ProjectID: i.Scope.Project,
					LocalSubject: account.Subject.ID, LinkID: link.ID, AuthorityID: state.reference,
					ProviderSessionID: state.reference, Generation: 1, LoginGeneration: 1,
					ExternalAuthTime: now.Add(-time.Minute), AbsoluteUntil: i.ExpiresAt,
					Deadline: now.Add(4*time.Minute), Proof: host.ExternalProof{IssuedAt: now.Add(-time.Second), ExpiresAt: i.ExpiresAt},
				}
				f.authority.mu.Lock()
				f.authority.bindings[state.reference] = b
				f.authority.mu.Unlock()
				if _, err := app.LoginExternalAdmin(c, b); err != nil { return err }
				return c.Redirect().To("/auth/profile")
			})
		}), nil
	}
}

// TestAdmissionRealBrowser owns one disposable database and one loopback TLS
// listener. The runner and SQL observations communicate over private OS pipes;
// there is no HTTP backdoor to grant roles, inspect storage or disable security.
func TestAdmissionRealBrowser(t *testing.T) {
	certPath, keyPath := os.Getenv("GOADMIN_BROWSER_TLS_CERT"), os.Getenv("GOADMIN_BROWSER_TLS_KEY")
	require.NotEmpty(t, certPath, "explicit trusted loopback TLS certificate required")
	require.NotEmpty(t, keyPath, "explicit TLS key path required")
	for _, key := range []string{"GOADMIN_BROWSER_PROFILE", "GOADMIN_BROWSER_EXECUTABLE", "GOADMIN_PLAYWRIGHT_MODULE"} {
		require.NotEmpty(t, os.Getenv(key), key+" is required; no installation or trust changes are performed")
	}
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	require.NoError(t, err)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	url := "https://localhost:"+strings.TrimPrefix(listener.Addr().String(), "127.0.0.1:")
	foreignListener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	foreignURL := "https://localhost:"+strings.TrimPrefix(foreignListener.Addr().String(), "127.0.0.1:")
	foreignServer := &http.Server{ReadHeaderTimeout: 5*time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<!doctype html><title>Synthetic foreign origin</title>"))
	})}
	foreignDone := make(chan error, 1)
	go func() { foreignDone <- foreignServer.Serve(tls.NewListener(foreignListener, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})) }()
	t.Cleanup(func() { require.NoError(t, foreignServer.Close()); require.ErrorIs(t, <-foreignDone, http.ErrServerClosed) })

	db, _, cleanup := tests.PrepareDB(t.Context(), t, "AdmissionBrowser")
	t.Cleanup(func() { cleanup(context.Background()) })
	cfg := host.Config{
		Admin: host.AdminConfig{Env: "staging", Domain: "localhost", DomainURL: url, BaseDomain: "localhost", BaseDomainURL: url, Addr: listener.Addr().String(),
			SessionName: "admission_browser_session", SessionTTL: time.Hour, SessionInactiveTTL: time.Hour, CSRFTokenName: "csrf_token", CSRFTokenTTL: time.Hour, CanonicalSessionBackend: "pgsql"},
		Server: host.ServerConfig{Addr: listener.Addr().String(), AllowOrigins: []string{url}, DisableStartupMessage: true},
		Auth: tests.AuthRuntimeConfig(t),
		CSRF: host.CSRFConfig{AppDomain: "localhost", SecretKey: uuid.NewString(), AllowedOrigins: []string{url}},
	}
	cfg.Auth.NotificationDelivery = goauth.NotificationDeliveryDisabled
	auth, err := host.NewAuthAdapter(host.AuthAdapterConfig{Database: db, TxManager: host.NewTxManager(db), Runtime: cfg.Auth})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, auth.Close()) })
	roles, err := auth.Runtime().RBAC(nil)
	require.NoError(t, err)
	readKey := rbac.MustPermissionKey("admission", "read")
	_, err = roles.UpsertPermission(t.Context(), rbac.Permission{Key: readKey})
	require.NoError(t, err)
	target, err := roles.CreateRole(t.Context(), rbac.Role{Slug: "browser-reader", Name: "Browser reader"}, []rbac.PermissionKey{readKey})
	require.NoError(t, err)
	keys := []rbac.PermissionKey{readKey, rbac.MustPermissionKey("admission", "approve"), rbac.MustPermissionKey("admission", "invite"), rbac.MustPermissionKey("admission", "reject"), rbac.MustPermissionKey("roles", "assign"), rbac.MustPermissionKey("admission", "grant-"+target.PublicID)}
	for _, key := range keys { _, err = roles.UpsertPermission(t.Context(), rbac.Permission{Key: key}); require.NoError(t, err) }
	manager, err := roles.CreateRole(t.Context(), rbac.Role{Slug: "browser-manager", Name: "Browser manager"}, keys)
	require.NoError(t, err)
	reader, err := roles.CreateRole(t.Context(), rbac.Role{Slug: "browser-observer", Name: "Browser observer"}, []rbac.PermissionKey{readKey})
	require.NoError(t, err)
	owner, err := roles.CreateRole(t.Context(), rbac.Role{Slug: "super_admin", Name: "Browser owner"}, keys)
	require.NoError(t, err)
	password := "Synthetic-Browser-"+uuid.NewString()+"!"
	for email, role := range map[string]int64{"manager@example.invalid": manager.ID, "observer@example.invalid": reader.ID, "owner@example.invalid": owner.ID} {
		account, err := auth.Runtime().ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{Email: email, Password: password})
		require.NoError(t, err)
		_, err = auth.Runtime().Database().ExecContext(t.Context(), "INSERT INTO administrations(subject_id,uuid) VALUES($1,$2)", account.Subject.ID, uuid.NewString())
		require.NoError(t, err)
		require.NoError(t, roles.ReplaceSubjectRoles(t.Context(), account.Subject.ID, []int64{role}))
	}
	authority := &browserAdmissionAuthority{scope: admission.Scope{Issuer: "https://synthetic-issuer.invalid", Project: "browser-project", Client: "browser-client"},
		identities: map[string]admission.VerifiedIdentity{}, bindings: map[string]host.ExternalBinding{}, states: map[string]browserAdmissionState{}, owners: map[string]string{}}
	component, err := admission.New(t.Context(), admission.Config{Runtime: auth.Runtime(), Mode: admission.ProjectPending, Scope: authority.scope, Verifier: authority,
		Reference: func(c fiber.Ctx) (string, error) {
			ref := c.Cookies("admission_fixture_reference")
			authority.mu.Lock()
			defer authority.mu.Unlock()
			if authority.owners[ref] == "" || authority.owners[ref] != c.Cookies("admission_fixture_browser") { return "", goauth.ErrSessionRevoked }
			return ref, nil
		}, SignInPath: "/admission-browser/start"})
	require.NoError(t, err)
	runtime, err := host.New(t.Context(), cfg, host.Dependencies{Database: db, Logger: host.DiscardLogger(), Auth: auth, ExternalAuthority: authority},
		access.New(), component.Module(), host.DefinedFeatureModule(host.ModuleDescriptor{Key: "admission-browser-fixture", RouteNamespaces: []string{"/admission-browser"}}, browserAdmissionFeature{authority, component, auth}))
	require.NoError(t, err)
	runCtx, cancel := context.WithCancel(t.Context())
	runDone := make(chan error, 1)
	go func() { runDone <- runtime.Run(runCtx, tls.NewListener(listener, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})) }()
	t.Cleanup(func() { cancel(); require.NoError(t, runtime.Close()); require.NoError(t, <-runDone) })
	require.Eventually(t, func() bool { return runtime.Readiness(t.Context()) == nil }, 5*time.Second, 20*time.Millisecond)

	ctx, stop := context.WithTimeout(t.Context(), 3*time.Minute)
	defer stop()
	cmd := exec.CommandContext(ctx, "node", "../../resources/scripts/admission-browser-acceptance.cjs")
	in, err := cmd.StdinPipe()
	require.NoError(t, err)
	out, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	encoder := json.NewEncoder(in)
	require.NoError(t, encoder.Encode(map[string]any{"url": url, "foreignURL": foreignURL, "password": password, "role": target.ID, "previewRole": manager.ID}))
	scanner := bufio.NewScanner(out)
	completed := false
	for scanner.Scan() {
		var message struct { Op string `json:"op"` }
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &message))
		if message.Op == "done" { completed = true; break }
		if message.Op == "failed" { break }
		if message.Op == "observations" {
			authority.mu.Lock()
			observations := append([]browserAdmissionObservation{}, authority.observations...)
			authority.mu.Unlock()
			require.NoError(t, encoder.Encode(observations))
			continue
		}
		require.Equal(t, "snapshot", message.Op)
		var raw []byte
		err := auth.Runtime().Database().QueryRowContext(t.Context(), `SELECT json_build_object(
			'records', COALESCE((SELECT json_agg(json_build_object('subject',external_subject,'status',status,'version',version)) FROM admin_admissions),'[]'::json),
			'invitations', COALESCE((SELECT json_agg(json_build_object('id',id,'subject',external_subject,'accepted',accepted_subject)) FROM admin_admission_invitations),'[]'::json),
			'memberships', (SELECT count(*) FROM administrations),
			'roles', (SELECT count(*) FROM auth_subject_roles),
			'sessions', (SELECT count(*) FROM auth_sessions),
			'credentials', (SELECT count(*) FROM auth_local_credentials),
			'links', (SELECT count(*) FROM auth_identity_links),
			'events', (SELECT count(*) FROM admin_admission_events))`).Scan(&raw)
		require.NoError(t, err)
		require.NoError(t, encoder.Encode(json.RawMessage(raw)))
	}
	require.NoError(t, scanner.Err())
	require.NoError(t, in.Close())
	require.NoError(t, cmd.Wait())
	require.True(t, completed, "runner must reach its terminal assertions")
}
