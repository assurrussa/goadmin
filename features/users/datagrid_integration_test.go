//go:build integration

package users_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	usersfeature "github.com/assurrussa/goadmin/features/users"
	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/tests"
)

func TestDataGridAuthenticatedUsersAndExport(t *testing.T) {
	db, _, cleanup := tests.PrepareDB(t.Context(), t, "DataGridHost")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cleanup(ctx)
	})
	const origin = "http://127.0.0.1:5185"
	const domain = "127.0.0.1"
	const password = "DataGrid-Unique-Password-2026!" //nolint:gosec // isolated synthetic test credential
	const setup = "datagrid-test-setup-token-32-bytes-only"
	token, err := host.NewFirstAdminSetupToken(setup)
	require.NoError(t, err)
	cfg := host.Config{
		Admin: host.AdminConfig{
			Env: "staging", Domain: domain, DomainURL: origin, BaseDomain: domain, BaseDomainURL: origin,
			Addr: "127.0.0.1:5185", SessionName: "datagrid_session", SessionTTL: time.Hour, SessionInactiveTTL: time.Minute,
			CSRFTokenName: "csrf_token", CSRFTokenTTL: time.Hour, CanonicalSessionBackend: "pgsql",
		},
		Server:               host.ServerConfig{Addr: "127.0.0.1:5185", AllowOrigins: []string{origin}, DisableStartupMessage: true},
		Auth:                 tests.AuthRuntimeConfig(t),
		CSRF:                 host.CSRFConfig{AppDomain: domain, SecretKey: strings.Repeat("s", 32), AllowedOrigins: []string{origin}},
		FirstAdminSetupToken: token,
	}
	cfg.Auth.NotificationDelivery = goauth.NotificationDeliveryDisabled
	runtime, err := host.New(t.Context(), cfg,
		host.Dependencies{Database: db, Logger: host.DiscardLogger()}, host.FeatureModule(usersfeature.NewFeature()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, runtime.Close()) })
	cookies := map[string]*http.Cookie{}
	request := func(method, path string, payload any, authenticated bool) (int, http.Header, []byte) {
		t.Helper()
		var body io.Reader
		if payload != nil {
			b, marshalErr := json.Marshal(payload)
			require.NoError(t, marshalErr)
			body = bytes.NewReader(b)
		}
		req := httptest.NewRequestWithContext(t.Context(), method, origin+path, body)
		if strings.HasPrefix(path, "/auth/") {
			req.Header.Set("X-Inertia", "true")
		}
		if strings.HasPrefix(path, "/users/export") {
			req.Header.Set("Accept", "text/csv")
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", origin)
		}
		if authenticated {
			for _, cookie := range cookies {
				req.AddCookie(cookie)
			}
			if c := cookies["csrf_token"]; c != nil {
				req.Header.Set("X-CSRF-Token", c.Value)
			}
		}
		response, reqErr := runtime.App().Test(req, fiber.TestConfig{Timeout: 10 * time.Second, FailOnTimeout: true})
		require.NoError(t, reqErr)
		b, readErr := io.ReadAll(response.Body)
		require.NoError(t, readErr)
		require.NoError(t, response.Body.Close())
		if authenticated {
			for _, cookie := range response.Cookies() {
				cookies[cookie.Name] = cookie
			}
		}
		return response.StatusCode, response.Header, b
	}
	for _, path := range []string{"/users/data", "/users/export"} {
		status, headers, body := request(http.MethodGet, path, nil, false)
		require.NotEqual(t, http.StatusOK, status, string(body))
		require.Empty(t, headers.Get("Content-Disposition"))
	}
	status, _, body := request(http.MethodGet, "/auth/login", nil, true)
	require.Equal(t, http.StatusOK, status, string(body))
	status, _, body = request(http.MethodPost, "/auth/register", map[string]string{
		"name": "DataGrid Admin", "username": "datagrid-admin", "email": "datagrid-admin@example.test",
		"password": password, "passwordConfirm": password, "setupToken": setup,
	}, true)
	require.Contains(t, []int{http.StatusFound, http.StatusSeeOther}, status, string(body))
	for i, name := range []string{"Alpha, \"quoted\"\nline", "Beta", "Gamma"} {
		account, createErr := runtime.Auth.Runtime().ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
			Email: fmt.Sprintf("datagrid-%d@example.test", i), Password: password,
			Profile: goauth.BasicProfile{GivenName: name, Username: fmt.Sprintf("fixture%d", i)},
		})
		require.NoError(t, createErr)
		_, err = db.DB().Pool().Exec(t.Context(),
			`INSERT INTO users(subject_id,uuid,created_at,updated_at) VALUES($1,$2,now(),now())`,
			account.Subject.ID, identity.NewUserID(),
		)
		require.NoError(t, err)
	}
	for _, query := range []string{
		"?page=1&limit=1&sortBy=name&sortOrder=asc",
		"?page=2&limit=1&search=datagrid&status=active&sortBy=name&sortOrder=desc",
		"?name=" + url.QueryEscape("Alpha") + "&limit=1",
		"?search=" + url.QueryEscape(`' OR true; DROP TABLE users; --`),
	} {
		status, _, body = request(http.MethodGet, "/users/data"+query, nil, true)
		require.Equal(t, http.StatusOK, status, string(body))
		var list struct {
			Data []struct {
				Item struct {
					ID   int64  `json:"id"`
					Name string `json:"name"`
				} `json:"item"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(body, &list))
		status, headers, body := request(http.MethodGet, "/users/export"+query, nil, true)
		require.Equal(t, http.StatusOK, status, string(body))
		require.Equal(t, "text/csv; charset=utf-8", headers.Get("Content-Type"))
		require.Equal(t, "no-store", headers.Get("Cache-Control"))
		records, parseErr := csv.NewReader(bytes.NewReader(body)).ReadAll()
		require.NoError(t, parseErr)
		switch {
		case strings.Contains(query, "DROP"):
			require.Empty(t, list.Data)
			require.Len(t, records, 1)
		case strings.Contains(query, "?name="):
			require.Len(t, list.Data, 1)
			require.Len(t, records, 2)
			require.Equal(t, "Alpha, \"quoted\"\nline", records[1][3])
		default:
			require.Len(t, list.Data, 1)
			require.Len(t, records, 4)
		}
	}
	// Explicit local browser harness. No real accounts, mail, or external services.
	// The default automated test never starts a listener.
	if os.Getenv("GOADMIN_DATAGRID_BROWSER") == "1" {
		t.Log("Synthetic browser host: http://127.0.0.1:5185/users")
		t.Log("Synthetic login: datagrid-admin@example.test / " + password)
		require.NoError(t, runtime.Run(t.Context()))
	}

	// Revoking roles must affect the same already-authenticated browser session.
	var subject goauth.SubjectID
	require.NoError(t, db.DB().Pool().QueryRow(t.Context(), "SELECT subject_id FROM administrations LIMIT 1").Scan(&subject))
	permissions, err := runtime.Auth.Runtime().RBAC(nil)
	require.NoError(t, err)
	require.NoError(t, permissions.ReplaceSubjectRoles(t.Context(), subject, nil))
	for _, path := range []string{"/users/data", "/users/export"} {
		status, headers, body := request(http.MethodGet, path, nil, true)
		require.Equal(t, http.StatusForbidden, status, string(body))
		require.Empty(t, headers.Get("Location"))
		require.Empty(t, headers.Get("Content-Disposition"))
	}
}
