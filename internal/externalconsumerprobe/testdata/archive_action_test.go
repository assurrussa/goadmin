package probe

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/postgres"
	"github.com/assurrussa/goauth/rbac"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/features/mcp"
	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/migrations"
)

const (
	archiveOrigin = "https://localhost:8080"
	archivePath   = "/agent/archive"
)

func archiveModule(t *testing.T) host.Module {
	t.Helper()
	action := mcp.Action{
		Namespace: "library", Key: "archive", Version: "1", Path: archivePath,
		Title: "Archive document", Description: "Propose archiving a document.", ConfirmLabel: "Archive",
		ProposeTool: "propose_archive", StatusTool: "get_archive_request",
		Permissions:  []host.PermissionKey{host.NewPermissionKey("documents", "archive")},
		Parameters:   []mcp.Parameter{{Name: "reference", Kind: mcp.Text, Required: true, Description: "Document reference."}},
		ResultFields: []mcp.Field{{Key: "archive_ref", Kind: mcp.Text, Description: "Archive reference."}},
		Normalize: func(_ string, params map[string]json.RawMessage) (json.RawMessage, error) {
			return json.Marshal(params)
		},
		Inspect: inspectDocument,
		Apply: func(ctx context.Context, tx mcp.ActionTx, _ host.Actor, _ json.RawMessage, doc mcp.ActionSnapshot, _ string) (map[string]string, error) {
			var ref string
			err := tx.QueryRow(ctx, "UPDATE library_documents SET archived=true WHERE reference=$1 AND NOT archived RETURNING reference", doc.RecordID).Scan(&ref)
			return map[string]string{"archive_ref": ref}, err
		},
	}
	module, err := mcp.ConfirmedModule(mcp.Config{Origin: archiveOrigin}, action)
	require.NoError(t, err)
	return module
}

func inspectDocument(ctx context.Context, tx mcp.ActionTx, _ host.Actor, raw json.RawMessage, lock bool) (mcp.ActionSnapshot, error) {
	var input struct{ Reference string }
	if err := json.Unmarshal(raw, &input); err != nil {
		return mcp.ActionSnapshot{}, err
	}
	query := "SELECT title, revision FROM library_documents WHERE reference=$1 AND NOT archived"
	if lock {
		query += " FOR UPDATE"
	}
	var title, revision string
	err := tx.QueryRow(ctx, query, input.Reference).Scan(&title, &revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return mcp.ActionSnapshot{}, mcp.ErrActionNotFound
	}
	return mcp.ActionSnapshot{
		RecordID: input.Reference, Revision: revision,
		Fields: []mcp.ReviewField{{Label: "Document", Value: title}},
	}, err
}

func TestConfirmedMCPAction(t *testing.T) {
	module := archiveModule(t)
	require.Equal(t, "mcp-action.library.archive", module.Descriptor().Key)
}

// The integration gate supplies its local PostgreSQL server. Only a newly
// created database is migrated and used by this external host.
func TestArchiveTransaction(t *testing.T) {
	server := os.Getenv("GOADMIN_PROBE_POSTGRES_URL")
	if server == "" {
		t.Skip("integration gate supplies GOADMIN_PROBE_POSTGRES_URL")
	}
	ctx := t.Context()
	dsn, err := url.Parse(server)
	require.NoError(t, err)
	admin, err := sql.Open("pgx", server)
	require.NoError(t, err)
	defer admin.Close()
	database := "archive_probe_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = admin.ExecContext(ctx, "CREATE DATABASE "+database)
	require.NoError(t, err)
	defer func() {
		_, dropErr := admin.ExecContext(context.Background(), "DROP DATABASE "+database+" WITH (FORCE)")
		require.NoError(t, dropErr)
	}()
	dsn.Path = "/" + database
	db, err := sql.Open("pgx", dsn.String())
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, migrations.Migrate(ctx, migrations.DatabaseConfig{DSN: dsn.String()}, db))
	_, err = db.ExecContext(ctx, "CREATE TABLE library_documents(reference text PRIMARY KEY, title text NOT NULL, revision text NOT NULL, archived boolean NOT NULL DEFAULT false)")
	require.NoError(t, err)
	const ref = "minutes/April"
	_, err = db.ExecContext(ctx, "INSERT INTO library_documents(reference,title,revision) VALUES ($1,'Board minutes','draft-A')", ref)
	require.NoError(t, err)

	log := host.DiscardLogger()
	pg, err := host.NewPgsqlClient(ctx, host.PgsqlConfig{DSN: dsn.String()}, log)
	require.NoError(t, err)
	defer pg.Close()
	adminCfg := host.BuildConfig(host.AdminConfigInput{
		Env: "production", Domain: "localhost", DomainURL: archiveOrigin,
		BaseDomain: "localhost", BaseDomainURL: archiveOrigin, Addr: ":8080", SessionName: "__Host-goadmin_session",
		CSRFTokenName: "csrf_token", CSRFTokenTTL: time.Hour, SessionTTL: time.Hour, SessionInactiveTTL: time.Hour,
	}, host.PathDefaults{})
	adminCfg.CanonicalSessionBackend = "pgsql"
	ring := func() goauth.KeyRing {
		key, keyErr := goauth.NewKeyRing("probe", goauth.Key{ID: "probe", Material: []byte(uuid.NewString()[:32])})
		require.NoError(t, keyErr)
		return key
	}
	cfg := host.Config{
		Admin: adminCfg, Server: host.ServerConfig{Addr: ":8080", AllowOrigins: []string{archiveOrigin}},
		CSRF: host.CSRFConfig{AppDomain: "localhost", SecretKey: uuid.NewString(), AllowedOrigins: []string{archiveOrigin}},
		Auth: goauth.Config{
			Signing:       goauth.SigningConfig{Issuer: archiveOrigin, Audience: "goadmin", Keys: ring()},
			TokenHMACKeys: ring(), OutboxAEADKeys: ring(), NotificationDelivery: goauth.NotificationDeliveryDisabled,
		},
	}
	csrf, err := host.NewCSRFService(cfg.CSRF, log)
	require.NoError(t, err)
	rt, err := host.New(ctx, cfg, host.Dependencies{Database: pg, TxManager: host.NewTxManager(pg), Logger: log, CSRF: csrf}, archiveModule(t))
	require.NoError(t, err)
	defer rt.Close()
	const email = "archivist@example.test"
	password := uuid.NewString() + "Ab!"
	account, err := rt.Auth.Runtime().ProvisionTrustedLocalAccount(ctx, goauth.RegisterRequest{Email: email, Password: password})
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "INSERT INTO administrations(subject_id,uuid) VALUES($1,gen_random_uuid())", account.Subject.ID.String())
	require.NoError(t, err)
	roles, err := postgres.NewRBAC(db, nil)
	require.NoError(t, err)
	role, err := roles.UpsertRole(ctx, rbac.Role{Slug: "super_admin", Name: "Archivist", System: true})
	require.NoError(t, err)
	_, err = roles.UpsertPermission(ctx, rbac.Permission{Key: "documents:archive"})
	require.NoError(t, err)
	require.NoError(t, roles.ReplaceRolePermissions(ctx, role.ID, []rbac.PermissionKey{"documents:archive"}))
	require.NoError(t, roles.ReplaceSubjectRoles(ctx, account.Subject.ID, []int64{role.ID}))

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	request := func(method, path string, payload map[string]string, browser bool, token string, want int) map[string]any {
		t.Helper()
		body, err := json.Marshal(payload)
		require.NoError(t, err)
		req := httptest.NewRequestWithContext(ctx, method, archiveOrigin+path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if browser {
			req.Header.Set("Origin", archiveOrigin)
			if method == http.MethodGet {
				req.Header.Set("X-Inertia", "true")
			}
			for _, c := range jar.Cookies(req.URL) {
				req.AddCookie(c)
				if c.Name == "csrf_token" {
					req.Header.Set("X-Csrf-Token", c.Value)
				}
			}
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := rt.App().Test(req, fiber.TestConfig{Timeout: 15 * time.Second})
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, want, resp.StatusCode, "%s %s (location %q)", method, path, resp.Header.Get("Location"))
		if browser {
			jar.SetCookies(req.URL, resp.Cookies())
		}
		var out map[string]any
		if strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
		}
		return out
	}
	request("GET", "/auth/login", nil, true, "", http.StatusOK)
	request("POST", "/auth/login", map[string]string{"email": email, "password": password}, true, "", http.StatusFound)
	pair := request("POST", archivePath+"/pair", map[string]string{}, false, "", http.StatusOK)
	request("GET", archivePath+"/connect", nil, true, "", http.StatusOK)
	request("POST", archivePath+"/connect", map[string]string{"code": pair["code"].(string), "decision": "approve"}, true, "", http.StatusOK)
	exchange := request("POST", archivePath+"/token", map[string]string{"device_secret": pair["device_secret"].(string)}, false, "", http.StatusOK)
	token := exchange["access_token"].(string)
	id := uuid.NewString()
	proposal := map[string]string{"request_id": id, "reference": ref}
	require.Equal(t, "pending", request("POST", archivePath+"/propose", proposal, false, token, http.StatusOK)["status"])
	var archived bool
	require.NoError(t, db.QueryRowContext(ctx, "SELECT archived FROM library_documents WHERE reference=$1", ref).Scan(&archived))
	require.False(t, archived)
	request("POST", archivePath+"/requests/"+id, map[string]string{"decision": "approve"}, true, "", http.StatusOK)
	receipt := request("POST", archivePath+"/status", map[string]string{"request_id": id}, false, token, http.StatusOK)
	require.Equal(t, "succeeded", receipt["status"])
	require.Equal(t, ref, receipt["archive_ref"])
	require.NotContains(t, receipt, "event_id")
	require.Equal(t, receipt, request("POST", archivePath+"/propose", proposal, false, token, http.StatusOK))
	// Both writes must also carry the same PostgreSQL transaction ID.
	var sameTx bool
	const committed = "SELECT d.archived, d.xmin=r.xmin FROM library_documents d JOIN admin_confirmed_actions r ON r.request_id=$1 WHERE d.reference=$2"
	require.NoError(t, db.QueryRowContext(ctx, committed, id, ref).Scan(&archived, &sameTx))
	require.True(t, archived)
	require.True(t, sameTx)
}
