//go:build integration

package sso //nolint:testpackage // Tests exercise internal binding and recovery invariants.

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	browser "github.com/assurrussa/gossoclient"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func testSSODatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not configured")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	base, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := "sport_sso_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = base.ExecContext(t.Context(), "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	db, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = base.ExecContext(ctx, "DROP DATABASE "+name+" WITH (FORCE)")
		_ = base.Close()
	})
	return db
}

const project = "11111111-1111-4111-8111-111111111111"

func testSSOIssuer(t *testing.T) (*httptest.Server, func(url.Values)) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	options := (&jose.SignerOptions{}).WithHeader("kid", "test").WithType("JWT")
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, options)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var nonce, challenge string
	issuer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := "https://" + r.Host
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer": origin, "authorization_endpoint": origin + "/authorize", "token_endpoint": origin + "/token",
				"jwks_uri": origin + "/keys", "revocation_endpoint": origin + "/revoke",
				"id_token_signing_alg_values_supported": []string{"RS256"},
			})
		case "/keys":
			keys := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
				{Key: &key.PublicKey, KeyID: "test", Algorithm: "RS256", Use: "sig"},
			}}
			_ = json.NewEncoder(w).Encode(keys)
		case "/token":
			if err := r.ParseForm(); err != nil {
				http.Error(w, "form", http.StatusBadRequest)
				return
			}
			id, secret, ok := r.BasicAuth()
			mu.Lock()
			n, ch := nonce, challenge
			mu.Unlock()
			hash := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if !ok || id != testClientID || secret != "synthetic-secret" || r.Form.Get(errorCodeProp) != "synthetic-code" ||
				base64.RawURLEncoding.EncodeToString(hash[:]) != ch {
				http.Error(w, "exchange denied", http.StatusBadRequest)
				return
			}
			now := time.Now().UTC().Truncate(time.Second)
			claims, _ := json.Marshal(map[string]any{
				"iss": origin, "sub": "subject-test", "aud": testClientID, "iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(),
				"auth_time": now.Add(-time.Minute).Unix(), "sid": "22222222-2222-4222-8222-222222222222",
				"project_id": project, "token_use": "id", "nonce": n, "session_expires_at": now.Add(time.Hour),
			})
			signed, e := signer.Sign(claims)
			if e != nil {
				http.Error(w, "sign", http.StatusInternalServerError)
				return
			}
			token, e := signed.CompactSerialize()
			if e != nil {
				http.Error(w, "serialize", http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "synthetic-access", "refresh_token": "synthetic-refresh",
				"token_type": "Bearer", "expires_in": 300, "id_token": token,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(issuer.Close)

	return issuer, func(values url.Values) {
		mu.Lock()
		nonce = values.Get("nonce")
		challenge = values.Get("code_challenge")
		mu.Unlock()
	}
}

func TestSSOReturnPathThroughRealSDK(t *testing.T) {
	db := testSSODatabase(t)
	if err := Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	issuer, setAuthorization := testSSOIssuer(t)
	var err error
	ca := filepath.Join(t.TempDir(), "issuer.pem")
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: issuer.Certificate().Raw})
	if err = os.WriteFile(ca, certificate, 0o600); err != nil {
		t.Fatal(err)
	}
	rp, err := NewRuntime(t.Context(), Config{
		Enabled: true, Origin: testAdminOrigin, Issuer: issuer.URL, ClientID: testClientID, ProjectID: project,
		ClientSecret: "synthetic-secret", StorageKey: base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 32))),
		HTTPSReverseProxy: true, IssuerCAFile: ca,
	}, db)
	if err != nil {
		t.Fatal(err)
	}
	f := &Feature{rp: rp}
	app := fiber.New()
	app.Get("/sso", f.page)
	app.Post("/login", f.begin)
	app.Get(callbackPath, f.callback)
	pageURL := rp.Config.Origin + "/sso?redirectback=%2Freports%3Ftab%3Dactive"
	page, err := app.Test(httptest.NewRequestWithContext(t.Context(), "GET", pageURL, nil))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(page.Body)
	_ = page.Body.Close()
	match := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindSubmatch(body)
	if len(match) != 2 {
		t.Fatalf("missing CSRF form: status %d", page.StatusCode)
	}
	form := url.Values{"csrf": {string(match[1])}, "return": {testReturnTarget}}
	req := httptest.NewRequestWithContext(t.Context(), "POST", rp.Config.Origin+"/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", rp.Config.Origin)
	for _, c := range page.Cookies() {
		req.AddCookie(c)
	}
	begin, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer begin.Body.Close()
	if begin.StatusCode != http.StatusSeeOther {
		t.Fatalf("begin status %d", begin.StatusCode)
	}
	authorization, err := url.Parse(begin.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	setAuthorization(authorization.Query())
	state := authorization.Query().Get("state")
	callbackURL := rp.Config.Origin + "/callback?code=synthetic-code&state=" + url.QueryEscape(state)
	callbackReq := httptest.NewRequestWithContext(t.Context(), "GET", callbackURL, nil)
	for _, c := range page.Cookies() {
		callbackReq.AddCookie(c)
	}
	callback, err := app.Test(callbackReq)
	if err != nil {
		t.Fatal(err)
	}
	defer callback.Body.Close()
	cbBody, _ := io.ReadAll(callback.Body)
	if callback.StatusCode != http.StatusOK || !strings.Contains(string(cbBody), continuationScript) {
		t.Fatalf("callback status %d: %s", callback.StatusCode, cbBody)
	}
	// Exercise the real SDK proof and the same JSONB intent round-trip used by
	// /sso-link. The old handler compared these Binding structs directly.
	verifyReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, rp.Config.Origin+"/sso-link", nil)
	allCookies := make(map[string]*http.Cookie)
	for _, cookie := range append(page.Cookies(), callback.Cookies()...) {
		allCookies[cookie.Name] = cookie
	}
	for _, cookie := range allCookies {
		verifyReq.AddCookie(cookie)
	}
	id, sdkBindingValue, err := rp.verify(verifyReq)
	if err != nil {
		t.Fatalf("real SDK proof not available after callback: %v", err)
	}
	local := goauth.NewSubjectID()
	snapshot, err := buildSnapshot(sdkBindingValue, id, local, "", time.Now())
	if err != nil {
		t.Fatalf("real SDK snapshot invalid: %v", err)
	}
	native := "native-integration-session"
	intent := strings.Repeat("z", 43)
	if err = rp.intents.create(t.Context(), intent, native, snapshot); err != nil {
		t.Fatal(err)
	}
	persisted, err := rp.intents.consume(t.Context(), intent, native, local)
	if err != nil {
		t.Fatal(err)
	}
	oldBinding, err := sdkBinding(persisted)
	if err != nil {
		t.Fatal(err)
	}
	current, err := buildSnapshot(sdkBindingValue, id, local, persisted.LinkID, time.Now())
	if err != nil || !sameSnapshot(current, persisted) {
		t.Fatalf("unchanged real SDK proof rejected by canonical comparison: %v", err)
	}
	t.Logf("real SDK direct Binding equality after PostgreSQL JSONB: %t; SDK deadline later than snapshot: %t",
		sdkBindingValue == oldBinding, sdkBindingValue.Deadline.After(persisted.Deadline))
	var ref string
	var generation int64
	var target string
	row := db.QueryRowContext(t.Context(), "SELECT authority_id,login_generation,target FROM goadmin_sso.sso_return_paths")
	if err = row.Scan(&ref, &generation, &target); err != nil {
		t.Fatal(err)
	}
	if ref == "" || generation <= 0 || target != testReturnTarget {
		t.Fatal("SDK round trip lost or unbound return target")
	}

	for _, wrong := range []browser.Binding{
		{Reference: "another-browser", LoginGeneration: generation},
		{Reference: ref, LoginGeneration: generation + 1},
	} {
		got, e := rp.takeReturn(t.Context(), wrong)
		if e != nil || got != "/" {
			t.Fatal("return target crossed browser/login binding", e)
		}
	}
	binding := browser.Binding{Reference: ref, LoginGeneration: generation}
	assertReturnRemainsAvailable(t, rp, binding)
	got, e := rp.takeReturn(t.Context(), binding)
	if e != nil || got != testReturnTarget {
		t.Fatal("bound return target unavailable", e)
	}
	got, e = rp.takeReturn(t.Context(), binding)
	if e != nil || got != "/" {
		t.Fatal("return target replayed", e)
	}
	var consumed bool
	query := "SELECT consumed_at IS NOT NULL FROM goadmin_sso.sso_login_return_intents WHERE state_digest=$1"
	if err = db.QueryRowContext(t.Context(), query, stateDigest(state)).Scan(&consumed); err != nil || !consumed {
		t.Fatal("login return intent not consumed", err)
	}
}

func assertReturnRemainsAvailable(t *testing.T, rp *Runtime, binding browser.Binding) {
	t.Helper()
	for range 2 {
		peeked, err := rp.peekReturn(t.Context(), binding)
		if err != nil || peeked != testReturnTarget {
			t.Fatal("peek consumed the target before admission", err)
		}
	}
}
