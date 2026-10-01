//go:build integration

package host //nolint:testpackage // exercises assembled runtime and transaction ownership

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/rbac"
	"github.com/stretchr/testify/require"

	adminrepo "github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminrepo"
	"github.com/assurrussa/goadmin/internal/admintx"
	"github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
	"github.com/assurrussa/goadmin/tests"
)

const (
	assemblyPassword    = "Assembly-Unique-Password-2026!"
	assemblyAdminDomain = "admin.test"
)

func assemblyFixture(t *testing.T) (Config, Dependencies) {
	t.Helper()
	db, _, cleanup := tests.PrepareDB(t.Context(), t, "MinimalAdminAssembly")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cleanup(ctx)
	})
	cfg := Config{
		Admin: AdminConfig{
			Env: "staging", Domain: assemblyAdminDomain, DomainURL: assemblyAdminOrigin,
			BaseDomain: assemblyAdminDomain, BaseDomainURL: assemblyAdminOrigin,
			Addr: "127.0.0.1:12345", SessionName: "assembly_session",
			SessionTTL: time.Hour, SessionInactiveTTL: time.Minute,
			CSRFTokenName: "csrf_token", CSRFTokenTTL: time.Hour,
			CanonicalSessionBackend: "pgsql",
		},
		Server: ServerConfig{Addr: "127.0.0.1:12345", AllowOrigins: []string{assemblyAdminOrigin}, DisableStartupMessage: true},
		Auth:   tests.AuthRuntimeConfig(t),
		CSRF: CSRFConfig{
			AppDomain: assemblyAdminDomain, SecretKey: strings.Repeat("s", 32),
			AllowedOrigins: []string{assemblyAdminOrigin},
		},
	}
	return cfg, Dependencies{Database: db, Logger: DiscardLogger()}
}

func TestAssemblyPreviewCASPreservesConcurrentMetadata(t *testing.T) {
	cfg, deps := assemblyFixture(t)
	cfg.Auth.NotificationDelivery = goauth.NotificationDeliveryDisabled
	adapter, err := NewAuthAdapter(AuthAdapterConfig{
		Database: deps.Database, TxManager: NewTxManager(deps.Database), Runtime: cfg.Auth,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adapter.Close()) })
	account, err := adapter.Runtime().ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
		Email: "preview-cas-assembly@example.test", Password: assemblyPassword,
	})
	require.NoError(t, err)
	admin, err := adapter.admins.ProvisionAccount(t.Context(), account, identity.NewUserID())
	require.NoError(t, err)
	loginAt := time.Now().UTC().Truncate(time.Millisecond)
	admin.Data = &models.AdminData{
		LastLoginAt: &loginAt, Roles: []string{"preserved-role"}, Permissions: map[string][]string{"profile": {"read"}},
	}
	require.NoError(t, adapter.admins.UpdateAdmin(t.Context(), admin.ID, admin))
	snapshot, err := adapter.admins.GetByID(t.Context(), admin.ID)
	require.NoError(t, err)
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, preview := range []int64{100, 200} {
		workers.Go(func() {
			<-start
			results <- adapter.Runtime().InAuthTransaction(t.Context(), func(ctx context.Context) error {
				executor, err := adapter.Runtime().SQLExecutor(ctx)
				if err != nil {
					return err
				}
				return adapter.admins.UpdatePreview(admintx.WithExecutor(ctx, executor, deps.Database.DB().Pool()), snapshot, preview)
			})
		})
	}
	close(start)
	workers.Wait()
	close(results)
	var success, conflict int
	for err := range results {
		if err == nil {
			success++
			continue
		}
		require.ErrorIs(t, err, adminrepo.ErrAdminVersionConflict)
		conflict++
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, conflict)
	current, err := adapter.admins.GetByID(t.Context(), admin.ID)
	require.NoError(t, err)
	require.Contains(t, []int64{100, 200}, current.GetPreviewFileID())
	require.Equal(t, snapshot.Data.LastLoginAt, current.Data.LastLoginAt)
	require.Equal(t, snapshot.Data.Roles, current.Data.Roles)
	require.Equal(t, snapshot.Data.Permissions, current.Data.Permissions)
	require.ErrorIs(t, adapter.admins.UpdatePreview(t.Context(), snapshot, 0), adminrepo.ErrAdminVersionConflict)
}

type assemblyPage struct {
	Component string                     `json:"component"`
	Props     map[string]json.RawMessage `json:"props"`
}

type assemblyHTTPResponse struct {
	StatusCode int
	Header     http.Header
}

func assemblyGet(
	t *testing.T, runtime *Runtime, path string, cookies map[string]*http.Cookie,
) (assemblyHTTPResponse, []byte) {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, assemblyAdminOrigin+path, nil)
	request.Header.Set("X-Inertia", "true")
	return assemblyRequest(t, runtime, request, cookies)
}

func assemblyRequest(
	t *testing.T, runtime *Runtime, request *http.Request, cookies map[string]*http.Cookie,
) (assemblyHTTPResponse, []byte) {
	t.Helper()
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response, err := runtime.App().Test(request)
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	for _, cookie := range response.Cookies() {
		cookies[cookie.Name] = cookie
	}
	return assemblyHTTPResponse{StatusCode: response.StatusCode, Header: response.Header}, body
}

func TestAssemblyMinimalPostgresLoginProfileAndSessionRestart(t *testing.T) {
	cfg, deps := assemblyFixture(t)
	first, err := New(t.Context(), cfg, deps)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, first.Close()) })
	require.Len(t, first.workers, 1, "the minimal core owns only session cleanup")
	require.Equal(t, "sessions", first.workers[0].name)
	account, err := first.Auth.Runtime().ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
		Email: "assembly-admin@example.test", Password: assemblyPassword,
		Profile: goauth.BasicProfile{Username: "operator", DisplayName: "Operator"},
	})
	require.NoError(t, err)
	_, err = first.Auth.admins.ProvisionAccount(t.Context(), account, identity.NewUserID())
	require.NoError(t, err)
	cookies := make(map[string]*http.Cookie)
	response, body := assemblyGet(t, first, "/auth/login", cookies)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	var login assemblyPage
	require.NoError(t, json.Unmarshal(body, &login))
	require.Equal(t, "auth/LoginPage", login.Component)
	var capabilities map[string]bool
	require.NoError(t, json.Unmarshal(login.Props["adminCapabilities"], &capabilities))
	require.NotNil(t, capabilities)
	require.Empty(t, capabilities)
	require.NotNil(t, cookies["csrf_token"])
	require.NotNil(t, cookies[cfg.Admin.SessionName])
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, assemblyAdminOrigin+"/auth/login",
		strings.NewReader(`{"email":"assembly-admin@example.test","password":"`+assemblyPassword+`","remember":true}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Inertia", "true")
	request.Header.Set("Origin", assemblyAdminOrigin)
	request.Header.Set("X-CSRF-Token", cookies["csrf_token"].Value)
	response, body = assemblyRequest(t, first, request, cookies)
	require.Contains(t, []int{http.StatusFound, http.StatusSeeOther}, response.StatusCode, string(body))

	response, body = assemblyGet(t, first, "/auth/profile", cookies)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	var profile assemblyPage
	require.NoError(t, json.Unmarshal(body, &profile))
	require.Equal(t, "profile/ViewPage", profile.Component)
	require.Contains(t, string(profile.Props["authuser"]), "assembly-admin@example.test")
	assertAssemblyOptionalProfileFields(t, first, cookies, account.Subject.ID)
	for _, path := range []string{
		"/admins", "/roles", "/permissions", "/files", "/queues", "/notifications", "/ws", "/auth/forgot-password",
	} {
		for _, route := range first.App().GetRoutes(true) {
			require.NotEqual(t, path, route.Path, "disabled module route must not be registered")
		}
	}
	require.NoError(t, first.Close())
	require.NoError(t, deps.Database.DB().Pool().Ping(t.Context()))
	second, err := New(t.Context(), cfg, deps)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, second.Close()) })
	response, body = assemblyGet(t, second, "/auth/profile", cookies)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	require.NoError(t, json.Unmarshal(body, &profile))
	require.Contains(t, string(profile.Props["authuser"]), "assembly-admin@example.test")
}

func assertAssemblyOptionalProfileFields(
	t *testing.T, runtime *Runtime, cookies map[string]*http.Cookie, subject goauth.SubjectID,
) {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, assemblyAdminOrigin+"/auth/profile/edit",
		strings.NewReader(`{"name":"Verified","lastName":null,"username":null}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Inertia", "true")
	request.Header.Set("Origin", assemblyAdminOrigin)
	request.Header.Set("Referer", assemblyAdminOrigin+"/auth/profile/edit")
	request.Header.Set("X-CSRF-Token", cookies["csrf_token"].Value)
	response, body := assemblyRequest(t, runtime, request, cookies)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	var page assemblyPage
	require.NoError(t, json.Unmarshal(body, &page))
	require.Equal(t, "profile/EditPage", page.Component)
	require.Contains(t, string(page.Props["authuser"]), "assembly-admin@example.test")
	account, err := runtime.Auth.Runtime().GetAccount(t.Context(), subject)
	require.NoError(t, err)
	require.Equal(t, "Verified", account.Profile.GivenName)
	require.Empty(t, account.Profile.FamilyName)
	require.Empty(t, account.Profile.Username)
}

func TestAssemblyCommandTransactionRollsBackCanonicalAndProjectionWrites(t *testing.T) {
	cfg, deps := assemblyFixture(t)
	cfg.Auth.NotificationDelivery = goauth.NotificationDeliveryDisabled
	adapter, err := NewAuthAdapter(AuthAdapterConfig{
		Database: deps.Database, TxManager: NewTxManager(deps.Database), Runtime: cfg.Auth,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adapter.Close()) })
	roles, err := adapter.Runtime().RBAC(nil)
	require.NoError(t, err)
	rollback := errors.New("injected projection failure")
	for _, abort := range []bool{true, false} {
		var subject goauth.SubjectID
		err = adapter.Runtime().InAuthTransaction(t.Context(), func(txCtx context.Context) error {
			executor, err := adapter.Runtime().SQLExecutor(txCtx)
			if err != nil {
				return err
			}
			ctx := admintx.WithExecutor(txCtx, executor, deps.Database.DB().Pool())
			account, err := adapter.Runtime().ProvisionTrustedLocalAccount(ctx, goauth.RegisterRequest{
				Email: "atomic-assembly@example.test", Password: assemblyPassword,
				Profile: goauth.BasicProfile{DisplayName: "Atomic Admin"},
			})
			if err != nil {
				return err
			}
			subject = account.Subject.ID
			if _, err = adapter.admins.ProvisionAccount(ctx, account, identity.NewUserID()); err != nil {
				return err
			}
			role, err := roles.UpsertRole(ctx, rbac.Role{Slug: "atomic-admin", Name: "Atomic admin"})
			if err != nil {
				return err
			}
			if err = roles.AssignRole(ctx, subject, role.Slug); err != nil {
				return err
			}
			if _, err = adapter.Runtime().LogoutAll(ctx, subject); err != nil {
				return err
			}
			if abort {
				return rollback
			}
			return nil
		})
		var canonical, projection, audit int
		db := adapter.Runtime().Database()
		require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM auth_subjects WHERE id=$1", subject).Scan(&canonical))
		require.NoError(t, db.QueryRowContext(t.Context(),
			"SELECT count(*) FROM administrations WHERE subject_id=$1", subject).Scan(&projection))
		require.NoError(t, db.QueryRowContext(t.Context(),
			"SELECT count(*) FROM auth_security_audit_events WHERE subject_id=$1", subject).Scan(&audit))
		if abort {
			require.ErrorIs(t, err, rollback)
			require.Zero(t, canonical)
			require.Zero(t, projection)
			require.Zero(t, audit)
		} else {
			require.NoError(t, err)
			require.Equal(t, 1, canonical)
			require.Equal(t, 1, projection)
			require.Positive(t, audit)
			assigned, err := roles.SubjectRoles(t.Context(), subject)
			require.NoError(t, err)
			require.Len(t, assigned, 1)
		}
	}
}

func TestAssemblyFirstAdminAuditFailureRollsBackBootstrap(t *testing.T) {
	cfg, deps := assemblyFixture(t)
	rawSetupToken := strings.Repeat("test-setup-", 4)
	var err error
	cfg.FirstAdminSetupToken, err = NewFirstAdminSetupToken(rawSetupToken)
	require.NoError(t, err)
	runtime, err := New(t.Context(), cfg, deps)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, runtime.Close()) })
	db := runtime.Auth.Runtime().Database()
	_, err = db.ExecContext(t.Context(), `CREATE FUNCTION reject_first_admin_test() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'injected first admin audit failure'; END; $$`)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `CREATE TRIGGER reject_first_admin_test BEFORE INSERT ON admin_action_audit
FOR EACH ROW EXECUTE FUNCTION reject_first_admin_test()`)
	require.NoError(t, err)
	cookies := make(map[string]*http.Cookie)
	postRegistration := func() {
		t.Helper()
		response, body := assemblyGet(t, runtime, "/auth/login", cookies)
		require.Equal(t, http.StatusOK, response.StatusCode, string(body))
		payload, err := json.Marshal(map[string]string{
			"name": "First", "username": "first-assembly", "email": "first-assembly@example.test",
			"password": assemblyPassword, "passwordConfirm": assemblyPassword, "setupToken": rawSetupToken,
		})
		require.NoError(t, err)
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
			assemblyAdminOrigin+"/auth/register", bytes.NewReader(payload))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Inertia", "true")
		request.Header.Set("Origin", assemblyAdminOrigin)
		request.Header.Set("X-CSRF-Token", cookies["csrf_token"].Value)
		response, _ = assemblyRequest(t, runtime, request, cookies)
		require.Contains(t, []int{http.StatusFound, http.StatusSeeOther}, response.StatusCode)
	}
	postRegistration()
	for _, table := range []string{"auth_subjects", "administrations", "admin_action_audit"} {
		var count int
		require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM "+table).Scan(&count))
		require.Zero(t, count, table+" must roll back after audit failure")
	}
	_, err = db.ExecContext(t.Context(), "DROP TRIGGER reject_first_admin_test ON admin_action_audit")
	require.NoError(t, err)
	postRegistration()
	var admins, audit int
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM administrations").Scan(&admins))
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT count(*) FROM admin_action_audit AS audit
JOIN administrations AS admin ON admin.id=audit.actor_admin_id
WHERE audit.action='admin.first_created' AND audit.actor_subject_id=admin.subject_id::text`).Scan(&audit))
	require.Equal(t, 1, admins)
	require.Equal(t, 1, audit)
	var cookieAuthority bool
	require.NoError(t, db.QueryRowContext(t.Context(),
		"SELECT EXISTS(SELECT 1 FROM goadmin_browser_auth_state WHERE id=$1)",
		cookies[cfg.Admin.SessionName].Value).Scan(&cookieAuthority))
	require.True(t, cookieAuthority, "bootstrap response must retain its authenticated browser authority")
	response, body := assemblyGet(t, runtime, "/auth/profile", cookies)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
}

func TestAssemblyFirstAdminPreservesExistingCanonicalAccount(t *testing.T) {
	cfg, deps := assemblyFixture(t)
	rawSetupToken := strings.Repeat("existing-setup-", 4)
	var err error
	cfg.FirstAdminSetupToken, err = NewFirstAdminSetupToken(rawSetupToken)
	require.NoError(t, err)
	runtime, err := New(t.Context(), cfg, deps)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, runtime.Close()) })
	auth := runtime.Auth.Runtime()
	const email = "existing-bootstrap@example.test"
	account, err := auth.ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
		Email: email, Password: assemblyPassword,
		Profile: goauth.BasicProfile{
			Username: "original-canonical", DisplayName: "Original Canonical", GivenName: "Original", FamilyName: "Canonical",
		},
	})
	require.NoError(t, err)
	_, err = auth.LogoutAll(t.Context(), account.Subject.ID)
	require.NoError(t, err)
	identifier := goauth.IdentifierInput{Scheme: goauth.IdentifierSchemeEmail, Value: email}
	original, err := auth.FindAccount(t.Context(), identifier)
	require.NoError(t, err)
	require.True(t, original.EmailVerified())
	require.Greater(t, original.Subject.SecurityVersion, account.Subject.SecurityVersion)
	cookies := make(map[string]*http.Cookie)
	post := func(password string) {
		t.Helper()
		response, body := assemblyGet(t, runtime, "/auth/login", cookies)
		require.Equal(t, http.StatusOK, response.StatusCode, string(body))
		payload, err := json.Marshal(map[string]string{
			"name": "Changed", "username": "changed-bootstrap", "lastName": "Changed", "email": email,
			"password": password, "passwordConfirm": password, "setupToken": rawSetupToken,
		})
		require.NoError(t, err)
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
			assemblyAdminOrigin+"/auth/register", bytes.NewReader(payload))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Inertia", "true")
		request.Header.Set("Origin", assemblyAdminOrigin)
		request.Header.Set("X-CSRF-Token", cookies["csrf_token"].Value)
		response, _ = assemblyRequest(t, runtime, request, cookies)
		require.Contains(t, []int{http.StatusFound, http.StatusSeeOther}, response.StatusCode)
	}
	assertUnchanged := func() {
		t.Helper()
		current, err := auth.FindAccount(t.Context(), identifier)
		require.NoError(t, err)
		require.Equal(t, original.Subject.ID, current.Subject.ID)
		require.Equal(t, original.Subject.SecurityVersion, current.Subject.SecurityVersion)
		require.Equal(t, original.Profile, current.Profile)
		require.Equal(t, original.PrimaryEmail, current.PrimaryEmail)
	}
	assertUnpromoted := func() {
		t.Helper()
		for _, table := range []string{"administrations", "auth_subject_roles", "admin_action_audit"} {
			var count int
			require.NoError(t, auth.Database().QueryRowContext(t.Context(), "SELECT count(*) FROM "+table).Scan(&count))
			require.Zero(t, count, table)
		}
		var consumed int
		require.NoError(t, auth.Database().QueryRowContext(t.Context(),
			"SELECT count(*) FROM goadmin_first_admin_setup_tokens WHERE consumed_at IS NOT NULL").Scan(&consumed))
		require.Zero(t, consumed, "failed bootstrap must not consume setup")
		assertUnchanged()
	}
	post("Wrong-Existing-Password-2026!")
	assertUnpromoted()
	_, err = auth.Database().ExecContext(t.Context(), `CREATE FUNCTION reject_existing_bootstrap() RETURNS trigger
LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected existing bootstrap audit failure'; END; $$;
CREATE TRIGGER reject_existing_bootstrap BEFORE INSERT ON admin_action_audit
FOR EACH ROW EXECUTE FUNCTION reject_existing_bootstrap()`)
	require.NoError(t, err)
	post(assemblyPassword)
	assertUnpromoted()
	_, err = auth.Database().ExecContext(t.Context(), "DROP TRIGGER reject_existing_bootstrap ON admin_action_audit")
	require.NoError(t, err)
	post(assemblyPassword)
	assertUnchanged()
	var canonical, admins, audit int
	db := auth.Database()
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM auth_subjects").Scan(&canonical))
	require.NoError(t, db.QueryRowContext(t.Context(),
		"SELECT count(*) FROM administrations WHERE subject_id=$1", original.Subject.ID).Scan(&admins))
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT count(*) FROM admin_action_audit AS audit
JOIN administrations AS admin ON admin.id=audit.actor_admin_id
WHERE audit.action='admin.first_created' AND audit.actor_subject_id=$1 AND admin.subject_id::text=$1`,
		original.Subject.ID).Scan(&audit))
	require.Equal(t, 1, canonical)
	require.Equal(t, 1, admins)
	require.Equal(t, 1, audit)
	roles, err := auth.RBAC(nil)
	require.NoError(t, err)
	assigned, err := roles.SubjectRoles(t.Context(), original.Subject.ID)
	require.NoError(t, err)
	require.Len(t, assigned, 1)
	require.Equal(t, "super_admin", assigned[0].Slug)
	response, body := assemblyGet(t, runtime, "/auth/profile", cookies)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	var profile assemblyPage
	require.NoError(t, json.Unmarshal(body, &profile))
	require.Equal(t, "profile/ViewPage", profile.Component)
	require.Contains(t, string(profile.Props["authuser"]), email)
}
