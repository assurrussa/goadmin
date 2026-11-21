//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/postgres"
	"github.com/assurrussa/goauth/rbac"

	"github.com/assurrussa/goadmin/host"
)

const unpublishTestOrigin = "http://localhost:18768"

type unpublishTestHTTP struct {
	t       *testing.T
	runtime *host.Runtime
}

type unpublishTestCookies map[string]*http.Cookie

func (s unpublishTestHTTP) request(method, path string, body any, jar unpublishTestCookies, headers http.Header, want int) []byte {
	s.t.Helper()
	var content []byte
	if body != nil {
		var err error
		content, err = json.Marshal(body)
		if err != nil {
			s.t.Fatal(err)
		}
	}
	req := httptest.NewRequestWithContext(s.t.Context(), method, unpublishTestOrigin+path, bytes.NewReader(content))
	for header, values := range headers {
		req.Header[header] = values
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, cookie := range jar {
		req.AddCookie(cookie)
	}
	resp, err := s.runtime.App().Test(req)
	if err != nil {
		s.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	for _, cookie := range resp.Cookies() {
		if jar != nil {
			if cookie.MaxAge < 0 {
				delete(jar, cookie.Name)
			} else {
				jar[cookie.Name] = cookie
			}
		}
	}
	if resp.StatusCode != want {
		s.t.Fatalf("%s %s: HTTP %d, want %d", method, path, resp.StatusCode, want)
	}
	result, err := io.ReadAll(resp.Body)
	if err != nil {
		s.t.Fatal(err)
	}
	return result
}

func (s unpublishTestHTTP) login(email string) unpublishTestCookies {
	s.t.Helper()
	jar := unpublishTestCookies{}
	s.request("GET", "/auth/login", nil, jar, http.Header{"X-Inertia": {"true"}}, http.StatusOK)
	s.request("POST", "/auth/login", map[string]string{"email": email, "password": "Test-only-Pairing-Secret-47!"}, jar, unpublishFormHeaders(jar), http.StatusFound)
	return jar
}

func unpublishFormHeaders(jar unpublishTestCookies) http.Header {
	return http.Header{"Origin": {unpublishTestOrigin}, "X-Csrf-Token": {jar["csrf_token"].Value}}
}

func (s unpublishTestHTTP) pair(jar unpublishTestCookies) string {
	s.t.Helper()
	var pairing struct {
		Code   string `json:"code"`
		Secret string `json:"device_secret"`
		URL    string `json:"verification_url"`
	}
	body := s.request("POST", unpublishAgentPath+"/pair", struct{}{}, nil, nil, http.StatusOK)
	if err := json.Unmarshal(body, &pairing); err != nil || pairing.URL != unpublishTestOrigin+unpublishConnectPath {
		s.t.Fatalf("pairing response could not be decoded: %v", err)
	}
	poll := map[string]string{"device_secret": pairing.Secret}
	s.request("POST", unpublishAgentPath+"/token", poll, nil, nil, http.StatusAccepted)
	s.request("GET", unpublishConnectPath, nil, jar, nil, http.StatusOK)
	s.request("POST", unpublishConnectPath, map[string]string{"code": pairing.Code, "decision": "approve"}, jar, unpublishFormHeaders(jar), http.StatusOK)
	var exchange struct {
		Token   string `json:"access_token"`
		Seconds int    `json:"expires_in"`
	}
	body = s.request("POST", unpublishAgentPath+"/token", poll, nil, nil, http.StatusOK)
	if err := json.Unmarshal(body, &exchange); err != nil || exchange.Token == "" || exchange.Seconds <= 0 || exchange.Seconds > 900 {
		s.t.Fatalf("exchange returned an invalid grant: %v", err)
	}
	return exchange.Token
}

func (s unpublishTestHTTP) agent(route, token string, input map[string]string, want int) unpublishOutput {
	s.t.Helper()
	body := s.request("POST", unpublishAgentPath+route, input, nil, http.Header{"Authorization": {"Bearer " + token}}, want)
	var result unpublishOutput
	if want == http.StatusOK {
		if err := json.Unmarshal(body, &result); err != nil {
			s.t.Fatal(err)
		}
	}
	return result
}

func TestProductUnpublishNativeHTTP(t *testing.T) {
	db, dsn, _ := productTestDatabase(t, "products,product-unpublish-mcp")
	ctx := t.Context()
	lg := host.DiscardLogger()
	pg, err := host.NewPgsqlClient(ctx, host.PgsqlConfig{DSN: dsn}, lg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pg.Close() })
	origin, _ := url.Parse(unpublishTestOrigin)
	admin := host.BuildConfig(host.AdminConfigInput{
		Env: "staging", Domain: origin.Hostname(), DomainURL: origin.String(),
		BaseDomain: origin.Hostname(), BaseDomainURL: origin.String(), Addr: ":8080",
		CSRFTokenName: "csrf_token", CSRFTokenTTL: time.Hour, SessionName: "goadmin_session",
		SessionTTL: time.Hour, SessionInactiveTTL: time.Hour,
	}, host.PathDefaults{})
	admin.CanonicalSessionBackend = "pgsql"
	cfg := host.Config{
		Admin: admin, Server: host.ServerConfig{Addr: ":8080", AllowOrigins: []string{origin.String()}},
		CSRF: host.CSRFConfig{AppDomain: origin.Hostname(), SecretKey: strings.Repeat("c", 32), AllowedOrigins: []string{origin.String()}},
		Auth: goauth.Config{
			Signing:              goauth.SigningConfig{Issuer: origin.String(), Audience: "goadmin"},
			NotificationDelivery: goauth.NotificationDeliveryDisabled,
		},
	}
	for name, target := range map[string]*goauth.KeyRing{
		"GOAUTH_SIGNING_KEY": &cfg.Auth.Signing.Keys, "GOAUTH_TOKEN_KEY": &cfg.Auth.TokenHMACKeys, "GOAUTH_ENVELOPE_KEY": &cfg.Auth.OutboxAEADKeys,
	} {
		*target, err = keyRing(name)
		if err != nil {
			t.Fatal(err)
		}
	}
	csrf, err := host.NewCSRFService(cfg.CSRF, lg)
	if err != nil {
		t.Fatal(err)
	}
	deps := host.Dependencies{Database: pg, TxManager: host.NewTxManager(pg), Logger: lg, CSRF: csrf}
	newHTTP := func() unpublishTestHTTP {
		t.Helper()
		products, err := productsModule(db)
		if err != nil {
			t.Fatal(err)
		}
		unpublish, err := productUnpublishModule(db, origin, netip.Addr{}, csrf, admin)
		if err != nil {
			t.Fatal(err)
		}
		runtime, err := host.New(ctx, cfg, deps, products, unpublish)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = runtime.Close() })
		return unpublishTestHTTP{t: t, runtime: runtime}
	}
	httpServer := newHTTP()
	roles, err := postgres.NewRBAC(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	const email = "unpublish-operator@example.test"
	account, err := httpServer.runtime.Auth.Runtime().ProvisionTrustedLocalAccount(ctx, goauth.RegisterRequest{
		Email: email, Password: "Test-only-Pairing-Secret-47!",
	})
	if err != nil {
		t.Fatal(err)
	}
	actor := host.Actor{SubjectID: account.Subject.ID.String()}
	if err := db.QueryRowContext(ctx, "INSERT INTO administrations(subject_id,uuid) VALUES($1,gen_random_uuid()) RETURNING id", actor.SubjectID).Scan(&actor.AdminID); err != nil {
		t.Fatal(err)
	}
	owner, err := roles.UpsertRole(ctx, rbac.Role{Slug: "super_admin", Name: "Administrator", System: true})
	if err != nil {
		t.Fatal(err)
	}
	permission := []rbac.PermissionKey{"products:unpublish"}
	if _, err := roles.UpsertPermission(ctx, rbac.Permission{Key: permission[0]}); err != nil {
		t.Fatal(err)
	}
	if err := roles.ReplaceRolePermissions(ctx, owner.ID, permission); err != nil {
		t.Fatal(err)
	}
	if err := roles.ReplaceSubjectRoles(ctx, account.Subject.ID, []int64{owner.ID}); err != nil {
		t.Fatal(err)
	}
	t.Run("effective grant stays locked through the product transaction", func(t *testing.T) {
		decision, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer decision.Rollback()
		if err := lockUnpublishAuthority(ctx, decision, actor); err != nil {
			t.Fatal(err)
		}
		short, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
		defer cancel()
		if err := roles.ReplaceRolePermissions(short, owner.ID, nil); err == nil || short.Err() == nil {
			t.Fatalf("concurrent revocation did not wait for the decision: %v", err)
		}
		if err := decision.Commit(); err != nil {
			t.Fatal(err)
		}
		if err := roles.ReplaceRolePermissions(ctx, owner.ID, nil); err != nil {
			t.Fatalf("revocation after commit: %v", err)
		}
		if err := roles.ReplaceRolePermissions(ctx, owner.ID, permission); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("logout can lock its subject while confirmation waits for the native login", func(t *testing.T) {
		h := &unpublishHandler{}
		browser := unpublishBrowser{actor: actor, csrf: host.CSRFRequest{SessionID: "fixture-login"}}
		endLogout := h.fences.acquire(browser.csrf.SessionID)
		var releaseOnce sync.Once
		defer releaseOnce.Do(endLogout)
		decision, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer decision.Rollback()
		finished := make(chan error, 1)
		go func() {
			release, err := h.lockBrowserAuthority(ctx, decision, browser)
			release()
			finished <- err
		}()
		until := time.Now().Add(3 * time.Second)
		for {
			h.fences.mu.Lock()
			waiting := h.fences.logins[browser.csrf.SessionID].refs == 2
			h.fences.mu.Unlock()
			if waiting {
				break
			}
			if time.Now().After(until) {
				t.Fatal("confirmation never reached the held login mutex")
			}
			time.Sleep(5 * time.Millisecond)
		}
		if _, err := db.ExecContext(ctx, "SELECT id FROM auth_subjects WHERE id=$1 FOR UPDATE NOWAIT", actor.SubjectID); err != nil {
			t.Fatalf("confirmation already holds the subject needed to finish logout: %v", err)
		}
		releaseOnce.Do(endLogout)
		if err := <-finished; err != nil {
			t.Fatal(err)
		}
	})
	repo := productRepo{db}
	publish := func() string {
		t.Helper()
		product, err := repo.create(ctx, actor, productInput{Name: "<Review product>"})
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.command(ctx, actor, product.ID, product.Revision, "publish"); err != nil {
			t.Fatal(err)
		}
		return product.ID
	}

	jar := httpServer.login(email)
	token := httpServer.pair(jar)
	other := httpServer.login(email)
	httpServer.request("GET", unpublishConnectPath, nil, other, nil, http.StatusOK)
	const requestID = "d1f23610-53d6-4262-bb60-c343b578c01c"
	path := unpublishRequestPath + requestID
	id := publish()
	proposal := map[string]string{"request_id": requestID, "product_id": id, "reason": "<script>immutable reason</script>"}
	statusInput := map[string]string{"request_id": requestID}
	httpServer.agent("/propose", "invalid", proposal, http.StatusUnauthorized)
	pending := httpServer.agent("/propose", token, proposal, http.StatusOK)
	if pending.Status != unpublishPending || pending.ReviewURL != unpublishTestOrigin+path {
		t.Fatalf("unexpected proposal status: %+v", pending)
	}
	if html := string(httpServer.request("GET", path, nil, jar, nil, http.StatusOK)); !strings.Contains(html, "&lt;script&gt;immutable reason&lt;/script&gt;") ||
		!strings.Contains(html, "&lt;Review product&gt;") || !strings.Contains(html, "Опубликован → Черновик") {
		t.Fatal("review did not escape and display the immutable proposal")
	}
	httpServer.request("GET", path, nil, other, nil, http.StatusNotFound)
	approval := map[string]string{"decision": "approve"}
	httpServer.request("POST", path, approval, other, unpublishFormHeaders(other), http.StatusNotFound)
	missingOrigin := unpublishFormHeaders(jar)
	missingOrigin.Del("Origin")
	missingOrigin.Set("Referer", unpublishTestOrigin+path)
	httpServer.request("POST", path, approval, jar, missingOrigin, http.StatusForbidden)
	httpServer.request("POST", path, approval, jar, http.Header{"Origin": {unpublishTestOrigin}}, http.StatusFound)
	if got := httpServer.agent("/status", token, statusInput, http.StatusOK); got.Status != unpublishPending {
		t.Fatalf("failed attempts or GET changed request to %q", got.Status)
	}
	for range 2 {
		httpServer.request("POST", path, approval, jar, unpublishFormHeaders(jar), http.StatusOK)
	}
	receipt := httpServer.agent("/status", token, statusInput, http.StatusOK)
	if receipt.Status != unpublishSucceeded || receipt.EventID == "" {
		t.Fatalf("invalid final receipt: %+v", receipt)
	}
	var events int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM starter_product_events WHERE action='unpublish' AND product_id=$1", id).Scan(&events); err != nil || events != 1 {
		t.Fatalf("unpublish event count=%d, error=%v", events, err)
	}

	pendingAfterRestart := map[string]string{"request_id": "d1f23610-53d6-4262-bb60-c343b578c02c"}
	httpServer.agent("/propose", token, map[string]string{"request_id": pendingAfterRestart["request_id"], "product_id": publish(), "reason": "restart"}, http.StatusOK)
	if err := httpServer.runtime.Close(); err != nil {
		t.Fatal(err)
	}
	httpServer = newHTTP()
	httpServer.agent("/status", token, statusInput, http.StatusUnauthorized)
	token = httpServer.pair(jar)
	if got := httpServer.agent("/status", token, statusInput, http.StatusOK); got != receipt {
		t.Fatalf("reconnect lost completed receipt: %+v", got)
	}
	if got := httpServer.agent("/status", token, pendingAfterRestart, http.StatusOK); got.Status != unpublishExpired {
		t.Fatalf("restart preserved an open request: %+v", got)
	}

	previewCandidate, err := roles.UpsertRole(ctx, rbac.Role{Slug: "view-only", Name: "View only"})
	if err != nil {
		t.Fatal(err)
	}
	pendingPreview := map[string]string{"request_id": "d1f23610-53d6-4262-bb60-c343b578c03c"}
	httpServer.agent("/propose", token, map[string]string{"request_id": pendingPreview["request_id"], "product_id": publish(), "reason": "preview"}, http.StatusOK)
	for _, roleID := range []int64{previewCandidate.ID, 0} {
		httpServer.request("POST", "/auth/role-preview", map[string]json.RawMessage{"roleId": json.RawMessage(strconv.FormatInt(roleID, 10))},
			jar, unpublishFormHeaders(jar), http.StatusOK)
	}
	httpServer.agent("/status", token, pendingPreview, http.StatusUnauthorized)
	token = httpServer.pair(jar)
	if got := httpServer.agent("/status", token, pendingPreview, http.StatusOK); got.Status != unpublishExpired {
		t.Fatalf("preview did not retire the pending review: %+v", got)
	}

	// Share native state with a second runtime, so the first runtime's
	// transition observer cannot know about this preview until it resolves it.
	peer := newHTTP()
	peer.request("POST", "/auth/role-preview", map[string]int64{"roleId": owner.ID}, jar, unpublishFormHeaders(jar), http.StatusOK)
	httpServer.agent("/status", token, statusInput, http.StatusForbidden)
	peer.request("POST", "/auth/role-preview", map[string]int64{"roleId": 0}, jar, unpublishFormHeaders(jar), http.StatusOK)
	token = httpServer.pair(jar)

	pendingPermission := map[string]string{"request_id": "d1f23610-53d6-4262-bb60-c343b578c04c"}
	httpServer.agent("/propose", token, map[string]string{"request_id": pendingPermission["request_id"], "product_id": publish(), "reason": "permission"}, http.StatusOK)
	if err := roles.ReplaceRolePermissions(ctx, owner.ID, nil); err != nil {
		t.Fatal(err)
	}
	httpServer.agent("/status", token, pendingPermission, http.StatusForbidden)
	if err := roles.ReplaceRolePermissions(ctx, owner.ID, permission); err != nil {
		t.Fatal(err)
	}
	token = httpServer.pair(jar)
	if got := httpServer.agent("/status", token, pendingPermission, http.StatusOK); got.Status != unpublishExpired {
		t.Fatalf("permission loss did not retire the pending review: %+v", got)
	}

	browserDenied := "d1f23610-53d6-4262-bb60-c343b578c06c"
	browserDeniedPath := unpublishRequestPath + browserDenied
	httpServer.agent("/propose", token, map[string]string{"request_id": browserDenied, "product_id": publish(), "reason": "browser lost permission"}, http.StatusOK)
	if err := roles.ReplaceRolePermissions(ctx, owner.ID, nil); err != nil {
		t.Fatal(err)
	}
	httpServer.request("GET", browserDeniedPath, nil, jar, nil, http.StatusForbidden)
	if err := roles.ReplaceRolePermissions(ctx, owner.ID, permission); err != nil {
		t.Fatal(err)
	}
	httpServer.agent("/status", token, map[string]string{"request_id": browserDenied}, http.StatusUnauthorized)
	token = httpServer.pair(jar)
	if got := httpServer.agent("/status", token, map[string]string{"request_id": browserDenied}, http.StatusOK); got.Status != unpublishExpired {
		t.Fatalf("restoring permission revived the denied browser review: %+v", got)
	}

	blockedID := "d1f23610-53d6-4262-bb60-c343b578c05c"
	blockedProduct := publish()
	httpServer.agent("/propose", token, map[string]string{"request_id": blockedID, "product_id": blockedProduct, "reason": "revoked"}, http.StatusOK)
	connectPage := httpServer.request("GET", unpublishConnectPath, nil, jar, nil, http.StatusOK)
	connectionID := regexp.MustCompile(`name="connection_id" value="([A-Z2-7]+)"`).FindSubmatch(connectPage)
	if len(connectionID) != 2 {
		t.Fatal("connection management did not render the active connection")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var locked string
	if err := tx.QueryRowContext(ctx, "SELECT id::text FROM starter_products WHERE id::text=$1 FOR UPDATE", blockedProduct).Scan(&locked); err != nil {
		t.Fatal(err)
	}
	review := httptest.NewRequestWithContext(ctx, http.MethodPost, unpublishTestOrigin+unpublishRequestPath+blockedID,
		strings.NewReader(`{"decision":"approve"}`))
	review.Header = unpublishFormHeaders(jar)
	review.Header.Set("Content-Type", "application/json")
	for _, cookie := range jar {
		review.AddCookie(cookie)
	}
	type pendingHTTP struct {
		status int
		result unpublishOutput
		err    error
	}
	finished := make(chan pendingHTTP, 1)
	go func() {
		resp, err := httpServer.runtime.App().Test(review)
		if err != nil {
			finished <- pendingHTTP{err: err}
			return
		}
		defer resp.Body.Close()
		var output unpublishOutput
		err = json.NewDecoder(resp.Body).Decode(&output)
		finished <- pendingHTTP{status: resp.StatusCode, result: output, err: err}
	}()
	waitUntil := time.Now().Add(750 * time.Millisecond)
	for {
		var waiting bool
		err := db.QueryRowContext(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock'
			AND query LIKE '%starter_products%FOR UPDATE%')`).Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if !time.Now().Before(waitUntil) {
			t.Fatal("browser confirmation did not wait on the product row lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	httpServer.request("POST", unpublishConnectPath, map[string]string{"decision": "revoke", "connection_id": string(connectionID[1])},
		jar, unpublishFormHeaders(jar), http.StatusOK)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-finished:
		if got.err != nil || got.status != http.StatusOK || got.result.Status != unpublishExpired {
			t.Fatalf("blocked confirmation survived revoke: %+v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("browser confirmation did not finish after releasing the product row lock")
	}
	product, err := repo.get(ctx, actor, blockedProduct)
	if err != nil || product.Data.Status != "published" {
		t.Fatalf("revoked confirmation changed product: %+v (%v)", product, err)
	}
}
