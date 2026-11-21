//go:build integration

package mcp //nolint:testpackage // Exercise the MCP grant and transport without exporting fixture hooks.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/rbac"
	logger "github.com/assurrussa/gologger"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/adminapp"
	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/hosttest"
	"github.com/assurrussa/goadmin/infrastructure/fiber/testsupport/utilst"
	inertiat "github.com/assurrussa/goadmin/infrastructure/inertia/testsupport"
	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminrepo"
	"github.com/assurrussa/goadmin/infrastructure/roles/adminroles"
	internalauth "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/services/adminservice"
	"github.com/assurrussa/goadmin/tests"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

func newAuthorizationFixture(t *testing.T) (*host.App, *internalauth.RoleService, *rbac.Service, host.Actor) {
	t.Helper()
	db, _, cleanup := tests.PrepareDB(t.Context(), t, "MCPGrantAuthorization")
	t.Cleanup(func() { cleanup(context.Background()) })
	tx := host.NewTxManager(db)
	repo := adminrepo.Must(adminrepo.NewOptions(db, tx))
	cfg := tests.AuthRuntimeConfig(t)
	cfg.NotificationDelivery = goauth.NotificationDeliveryDisabled
	authority := func(cache *internalauth.AuthorizationCacheConfig) *internalauth.Adapter {
		t.Helper()
		auth, err := internalauth.New(internalauth.Config{
			Database: db, Memberships: repo, Runtime: cfg, AuthorizationCache: cache,
		})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, auth.Close()) })
		return auth
	}
	local := authority(&internalauth.AuthorizationCacheConfig{
		RefreshInterval: time.Hour, MaxAge: 2 * time.Hour,
	})
	// A separate Runtime uses another SQL handle and no shared invalidator.
	external := authority(nil)
	require.NotSame(t, local.Runtime().Database(), external.Runtime().Database())
	externalRBAC, err := external.Runtime().RBAC(nil)
	require.NoError(t, err)
	operator, err := local.Runtime().ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
		Email: "mcp-cache-operator@example.test", Password: "Q7!McpCache_" + identity.NewUserID().String(),
	})
	require.NoError(t, err)
	admin, err := repo.ProvisionAccount(t.Context(), operator, identity.NewUserID())
	require.NoError(t, err)
	guard, err := internalauth.NewGuardService(
		internalauth.MustListSubjectRolesUseCase(local.Roles()), local.Roles(), logger.Discard(),
	)
	require.NoError(t, err)
	roles := adminroles.Must(guard, repo)
	store := utilst.NewTestStore()
	app := adminapp.Must(adminapp.NewOptions(
		inertiat.NewForTest(""), roles, roles, adminservice.NewService(store, roles),
		store, nil, nil, tx, logger.Discard(), "stage", "https://files.example.test",
	))
	return host.WrapApp(app), local.Roles(), externalRBAC, host.Actor{AdminID: admin.ID, SubjectID: operator.Subject.ID.String()}
}

func TestDelegatedReadUsesCanonicalPermissionBehindStaleWebSnapshot(t *testing.T) {
	hostApp, local, external, owner := newAuthorizationFixture(t)
	resource := testResource()
	key := resource.Permission
	require.NoError(t, local.EnsurePermissions(t.Context(), []internalauth.CreatePermissionInput{{Key: key}}))
	role, err := local.CreateRole(t.Context(), internalauth.CreateRoleInput{
		Slug: "delegated-reader", Name: "Delegated reader", Permissions: []internalauth.PermissionKey{key},
	})
	require.NoError(t, err)
	require.NoError(t, local.AssignRolesToSubject(t.Context(), owner.SubjectID, []int64{role.ID}))
	webAccess := func() bool {
		return hostApp.Unwrap().RolesService().AdminCan(t.Context(), owner.AdminID, key.Domain, key.Action)
	}
	require.Eventually(t, webAccess, 5*time.Second, 10*time.Millisecond, "warm the ordinary browser snapshot")

	const origin = "https://admin.example.test"
	const recordValue = "canonical private record"
	row := Record{ID: "12", Data: map[string]any{
		testGroupField: "9", testTitleField: recordValue,
	}}
	h := newHandler(hostApp, Config{Origin: origin}, resource, ReadFuncs{
		ListFunc: func(context.Context, host.Actor, datagrid.Filters) ([]Record, int, error) {
			return []Record{row}, 1, nil
		},
		GetFunc: func(context.Context, host.Actor, string) (Record, error) { return row, nil },
	})
	h.resolveRead = func(ctx context.Context, _ host.ReadSessionBinding) (context.Context, host.Actor, error) {
		return ctx, owner, nil
	}
	app := fiber.New()
	h.registerDevice(app)
	authenticate, err := hosttest.WithActor(owner)
	require.NoError(t, err)
	app.Use(h.connectPath(), authenticate)
	h.registerBrowser(app)
	source := netip.MustParseAddr("127.0.0.1")
	approvedCode, deviceSecret, ok := h.grants.start(source)
	require.True(t, ok)
	require.True(t, h.grants.decide(approvedCode, true, owner, host.ReadSessionBinding{}))
	token, _, status := h.grants.exchange(deviceSecret)
	require.Equal(t, http.StatusOK, status)
	request := func(path, data string, browser bool, expected int) string {
		t.Helper()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, origin+path, strings.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		if browser {
			req.Header.Set("Origin", origin)
		} else {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, expected, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return string(body)
	}
	const query = `{"group":"9"}`
	const id = `{"reading_id":"12"}`
	require.Contains(t, request(resource.Path+"/search", query, false, http.StatusOK), recordValue)
	require.Contains(t, request(resource.Path+"/get", id, false, http.StatusOK), recordValue)

	require.NoError(t, external.ReplaceRolePermissions(t.Context(), role.ID, nil))
	require.True(t, webAccess(), "the independent commit must leave the warmed web snapshot stale")
	require.NotContains(t, request(resource.Path+"/search", query, false, http.StatusForbidden), recordValue)
	require.NotContains(t, request(resource.Path+"/get", id, false, http.StatusForbidden), recordValue)

	connect := func(expected int) []byte {
		t.Helper()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, origin+h.connectPath(), nil)
		req.Header.Set("X-Inertia", "true")
		page, err := app.Test(req)
		require.NoError(t, err)
		defer page.Body.Close()
		require.Equal(t, expected, page.StatusCode)
		data, err := io.ReadAll(page.Body)
		require.NoError(t, err)
		return data
	}
	require.Contains(t, string(connect(http.StatusOK)), approvedCode)

	waiting, waitingSecret, ok := h.grants.start(source)
	require.True(t, ok)
	decide := func(code, decision string, expected int) {
		t.Helper()
		request(h.connectPath(), fmt.Sprintf(`{"code":%q,"decision":%q}`, code, decision), true, expected)
	}
	decide(waiting, "approve", http.StatusForbidden)
	_, _, status = h.grants.exchange(waitingSecret)
	require.Equal(t, http.StatusAccepted, status)
	decide(waiting, "deny", http.StatusOK)
	decide(approvedCode, "revoke", http.StatusOK)
	connect(http.StatusForbidden)
	request(resource.Path+"/search", query, false, http.StatusUnauthorized)
	require.True(t, webAccess(), "canonical grant checks do not invalidate ordinary web authorization")
}
