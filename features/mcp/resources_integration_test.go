//go:build integration

package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/rbac"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/features/mcp"
	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminrepo"
	"github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/tests"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

const (
	resourcesDomain = "admin.example.test"
	resourcesOrigin = "https://" + resourcesDomain
	resourcesCSRF   = "csrf_token"
	resourcesEmail  = "mcp-resource-reader@example.test"
)

func readOnlyResourceModule(t *testing.T, name, title string) (host.Module, mcp.Resource) {
	t.Helper()
	resource := mcp.Resource{
		Path: "/agent/" + name, Name: name, Description: "Read " + name,
		Permission:          host.NewPermissionKey(host.PermissionDomain(name), host.PermissionActionRead),
		RecordIDDescription: "Record ID",
		Search: &mcp.Search{
			ToolName: "search_" + name, Description: "Search " + name,
			Fields: []mcp.Field{{Key: "title", Description: "Record title", Kind: mcp.Text}},
		},
	}
	module, err := mcp.Module(mcp.Config{Origin: resourcesOrigin}, resource, mcp.ReadFuncs{
		ListFunc: func(context.Context, host.Actor, datagrid.Filters) ([]mcp.Record, int, error) {
			return []mcp.Record{{ID: "1", Data: map[string]any{"title": title}}}, 1, nil
		},
	})
	require.NoError(t, err)
	return module, resource
}

func newReadOnlyResourcesRuntime(t *testing.T, modules ...host.Module) (*host.Runtime, goauth.SubjectID, string) {
	t.Helper()
	db, _, cleanup := tests.PrepareDB(t.Context(), t, "MCPReadOnlyResources")
	t.Cleanup(func() { cleanup(context.Background()) })
	auth := tests.AuthRuntimeConfig(t)
	auth.NotificationDelivery = goauth.NotificationDeliveryDisabled
	cfg := host.Config{
		Admin: host.AdminConfig{
			Env: "staging", Domain: resourcesDomain, DomainURL: resourcesOrigin,
			BaseDomain: resourcesDomain, BaseDomainURL: resourcesOrigin,
			Addr: "127.0.0.1:12345", SessionName: "resources_session",
			SessionTTL: time.Hour, SessionInactiveTTL: time.Minute,
			CSRFTokenName: resourcesCSRF, CSRFTokenTTL: time.Hour, CanonicalSessionBackend: "pgsql",
		},
		Server: host.ServerConfig{Addr: "127.0.0.1:12345", AllowOrigins: []string{resourcesOrigin}, DisableStartupMessage: true},
		Auth:   auth,
		CSRF: host.CSRFConfig{
			AppDomain: resourcesDomain, SecretKey: strings.Repeat("s", 32), AllowedOrigins: []string{resourcesOrigin},
		},
	}
	tx := host.NewTxManager(db)
	runtime, err := host.New(t.Context(), cfg, host.Dependencies{Database: db, TxManager: tx, Logger: host.DiscardLogger()}, modules...)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, runtime.Close()) })
	password := "Q7!McpResources_" + identity.NewUserID().String()
	operator, err := runtime.Auth.Runtime().ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
		Email: resourcesEmail, Password: password,
	})
	require.NoError(t, err)
	repo := adminrepo.Must(adminrepo.NewOptions(db, tx))
	_, err = repo.ProvisionAccount(t.Context(), operator, identity.NewUserID())
	require.NoError(t, err)
	return runtime, operator.Subject.ID, password
}

func TestReadOnlyResourcesKeepIndependentGrantsUntilBrowserLogout(t *testing.T) {
	const (
		productsTitle = "Product entry"
		ordersTitle   = "Order entry"
		loginPath     = "/auth/login"
	)
	productsModule, products := readOnlyResourceModule(t, "products", productsTitle)
	ordersModule, orders := readOnlyResourceModule(t, "orders", ordersTitle)
	runtime, subject, password := newReadOnlyResourcesRuntime(t, productsModule, ordersModule)
	roles, err := runtime.Auth.Runtime().RBAC(nil)
	require.NoError(t, err)
	allowRead := func(resource mcp.Resource) {
		t.Helper()
		key := rbac.PermissionKey(resource.Permission.String())
		_, err := roles.UpsertPermission(t.Context(), rbac.Permission{Key: key})
		require.NoError(t, err)
		slug := resource.Name + "-reader"
		_, err = roles.CreateRole(t.Context(), rbac.Role{Slug: slug, Name: slug}, []rbac.PermissionKey{key})
		require.NoError(t, err)
		require.NoError(t, roles.AssignRole(t.Context(), subject, slug))
	}
	allowRead(products)

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	request := func(req *http.Request) (int, http.Header, []byte) {
		t.Helper()
		resp, err := runtime.App().Test(req, fiber.TestConfig{Timeout: 0, FailOnTimeout: false})
		require.NoError(t, err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, resp.Header, body
	}
	browser := func(method, path, body string) (int, http.Header, []byte) {
		t.Helper()
		req := httptest.NewRequestWithContext(t.Context(), method, resourcesOrigin+path, strings.NewReader(body))
		for _, cookie := range jar.Cookies(req.URL) {
			req.AddCookie(cookie)
			if cookie.Name == resourcesCSRF {
				req.Header.Set("X-CSRF-Token", cookie.Value)
			}
		}
		if path == "/auth/logout" {
			req.Header.Set("X-Inertia", "true")
		}
		if method != http.MethodGet {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", resourcesOrigin)
		}
		if strings.HasSuffix(path, "/connect") && method == http.MethodPost {
			req.Header.Set("Accept", "application/json")
		}
		status, headers, response := request(req)
		jar.SetCookies(req.URL, (&http.Response{Header: headers}).Cookies())
		return status, headers, response
	}
	device := func(path, input, bearer string, want int) []byte {
		t.Helper()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, resourcesOrigin+path, strings.NewReader(input))
		req.Header.Set("Content-Type", "application/json")
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		status, _, body := request(req)
		require.Equal(t, want, status, "%s: %s", path, body)
		return body
	}
	status, _, body := browser(http.MethodGet, loginPath, "")
	require.Equal(t, http.StatusOK, status, string(body))
	credentials := fmt.Sprintf(`{"email":%q,"password":%q}`, resourcesEmail, password)
	status, _, body = browser(http.MethodPost, loginPath, credentials)
	require.Contains(t, []int{http.StatusFound, http.StatusSeeOther}, status, string(body))
	status, _, body = browser(http.MethodGet, orders.Path+"/connect", "")
	require.Equal(t, http.StatusForbidden, status, string(body))

	type pairing struct {
		Code            string `json:"code"`
		DeviceSecret    string `json:"device_secret"`    //nolint:tagliatelle // Public device protocol.
		VerificationURL string `json:"verification_url"` //nolint:tagliatelle // Public device protocol.
	}
	start := func(resource mcp.Resource) pairing {
		t.Helper()
		var pair pairing
		require.NoError(t, json.Unmarshal(device(resource.Path+"/pair", "{}", "", http.StatusOK), &pair))
		require.NotEmpty(t, pair.Code)
		require.NotEmpty(t, pair.DeviceSecret)
		require.Equal(t, resourcesOrigin+resource.Path+"/connect", pair.VerificationURL)
		return pair
	}
	decide := func(resource mcp.Resource, pair pairing, decision string, grants ...string) {
		t.Helper()
		connectPath := resource.Path + "/connect"
		status, _, body := browser(http.MethodGet, connectPath, "")
		require.Equal(t, http.StatusOK, status, string(body))
		input := fmt.Sprintf(`{"code":%q,"decision":%q}`, pair.Code, decision)
		status, _, body = browser(http.MethodPost, connectPath, input)
		require.Equal(t, http.StatusOK, status, string(body))
		var result struct {
			Grants []string `json:"grants"`
		}
		require.NoError(t, json.Unmarshal(body, &result))
		require.ElementsMatch(t, grants, result.Grants)
	}
	exchange := func(resource mcp.Resource, pair pairing, want int) []byte {
		t.Helper()
		input := fmt.Sprintf(`{"device_secret":%q}`, pair.DeviceSecret)
		return device(resource.Path+"/token", input, "", want)
	}
	token := func(resource mcp.Resource, pair pairing) string {
		t.Helper()
		var result struct {
			AccessToken string `json:"access_token"` //nolint:tagliatelle // Public device protocol.
		}
		require.NoError(t, json.Unmarshal(exchange(resource, pair, http.StatusOK), &result))
		require.NotEmpty(t, result.AccessToken)
		return result.AccessToken
	}
	read := func(resource mcp.Resource, bearer, title string) {
		t.Helper()
		expected := fmt.Sprintf(`{"rows":[{"id":"1","title":%q}],"total":1}`, title)
		require.JSONEq(t, expected, string(device(resource.Path+"/search", "{}", bearer, http.StatusOK)))
	}
	requireClosed := func(resource mcp.Resource, bearer string) {
		t.Helper()
		body := device(resource.Path+"/search", "{}", bearer, http.StatusUnauthorized)
		require.JSONEq(t, `{"error":"authorization_required"}`, string(body))
	}

	allowRead(orders)
	productsPair, ordersPair := start(products), start(orders)
	decide(products, productsPair, "approve", productsPair.Code)
	productsToken := token(products, productsPair)
	require.JSONEq(t, `{"error":"authorization_pending"}`, string(exchange(orders, ordersPair, http.StatusAccepted)))
	decide(orders, ordersPair, "approve", ordersPair.Code)
	ordersToken := token(orders, ordersPair)
	read(products, productsToken, productsTitle)
	read(orders, ordersToken, ordersTitle)

	decide(products, productsPair, "revoke")
	requireClosed(products, productsToken)
	read(orders, ordersToken, ordersTitle)

	replacement := start(products)
	decide(products, replacement, "approve", replacement.Code)
	replacementToken := token(products, replacement)
	requireClosed(products, productsToken)
	read(products, replacementToken, productsTitle)
	read(orders, ordersToken, ordersTitle)

	status, header, body := browser(http.MethodDelete, "/auth/logout", "")
	require.Contains(t, []int{http.StatusFound, http.StatusSeeOther}, status, string(body))
	require.Equal(t, loginPath, header.Get("Location"))
	requireClosed(products, replacementToken)
	requireClosed(orders, ordersToken)
}
