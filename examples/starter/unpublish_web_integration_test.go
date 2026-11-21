//go:build integration

package main

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/postgres"
	"github.com/assurrussa/goauth/rbac"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"

	"github.com/assurrussa/goadmin/host"
)

const (
	unpublishTestOrigin      = "http://localhost:18768"
	unpublishTestSessionName = "goadmin_session"
)

const (
	unpublishConnectPath = unpublishAgentPath + "/connect"
	unpublishRequestPath = unpublishAgentPath + "/requests/"
	unpublishPending     = "pending"
	unpublishExpired     = "expired"
	unpublishSucceeded   = "succeeded"
)

type unpublishOutput struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
	ReviewURL string `json:"review_url,omitempty"`
	EventID   string `json:"event_id,omitempty"`
}

type unpublishTestHTTP struct {
	t       *testing.T
	runtime *host.Runtime
	origin  string
	do      func(*http.Request) (*http.Response, error)
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
	req, err := http.NewRequestWithContext(s.t.Context(), method, s.origin+path, bytes.NewReader(content))
	if err != nil {
		s.t.Fatal(err)
	}
	for header, values := range headers {
		req.Header[header] = values
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, cookie := range jar {
		req.AddCookie(cookie)
	}
	resp, err := s.do(req)
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
	s.request("POST", "/auth/login", map[string]string{"email": email, "password": "Test-only-Pairing-Secret-47!"}, jar, s.formHeaders(jar), http.StatusFound)
	return jar
}

func (s unpublishTestHTTP) formHeaders(jar unpublishTestCookies) http.Header {
	s.t.Helper()
	csrf := jar["csrf_token"]
	if csrf == nil || csrf.Value == "" {
		s.t.Fatal("browser did not receive a CSRF cookie")
	}
	return http.Header{"Origin": {s.origin}, "X-Csrf-Token": {csrf.Value}}
}

func (s unpublishTestHTTP) pair(jar unpublishTestCookies) string {
	s.t.Helper()
	var pairing struct {
		Code   string `json:"code"`
		Secret string `json:"device_secret"`
		URL    string `json:"verification_url"`
	}
	body := s.request("POST", unpublishAgentPath+"/pair", struct{}{}, nil, nil, http.StatusOK)
	if err := json.Unmarshal(body, &pairing); err != nil || pairing.URL != s.origin+unpublishConnectPath {
		s.t.Fatalf("pairing response could not be decoded: %v", err)
	}
	poll := map[string]string{"device_secret": pairing.Secret}
	s.request("POST", unpublishAgentPath+"/token", poll, nil, nil, http.StatusAccepted)
	s.request("GET", unpublishConnectPath, nil, jar, nil, http.StatusOK)
	s.request("POST", unpublishConnectPath, map[string]string{"code": pairing.Code, "decision": "approve"}, jar, s.formHeaders(jar), http.StatusOK)
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
	newHTTP := func(t *testing.T, rawOrigin string) unpublishTestHTTP {
		t.Helper()
		origin, err := url.Parse(rawOrigin)
		if err != nil {
			t.Fatal(err)
		}
		admin := starterAdminConfig(origin, host.AdminConfigInput{
			Domain: origin.Hostname(), DomainURL: rawOrigin,
			BaseDomain: origin.Hostname(), BaseDomainURL: rawOrigin, Addr: ":8080",
			CSRFTokenName: "csrf_token", CSRFTokenTTL: time.Hour,
			SessionTTL: time.Hour, SessionInactiveTTL: time.Hour,
		})
		admin.CanonicalSessionBackend = "pgsql"
		cfg := host.Config{
			Admin: admin, Server: host.ServerConfig{Addr: ":8080", AllowOrigins: []string{rawOrigin}},
			CSRF: host.CSRFConfig{AppDomain: origin.Hostname(), SecretKey: strings.Repeat("c", 32), AllowedOrigins: []string{rawOrigin}},
			Auth: goauth.Config{
				Signing:              goauth.SigningConfig{Issuer: rawOrigin, Audience: "goadmin"},
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
		products, err := productsModule(db)
		if err != nil {
			t.Fatal(err)
		}
		unpublish, err := productUnpublishModule(origin, netip.Addr{})
		if err != nil {
			t.Fatal(err)
		}
		runtime, err := host.New(ctx, cfg, deps, products, unpublish)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = runtime.Close() })
		return unpublishTestHTTP{t: t, runtime: runtime, origin: rawOrigin, do: func(req *http.Request) (*http.Response, error) {
			return runtime.App().Test(req, fiber.TestConfig{Timeout: 15 * time.Second})
		}}
	}
	httpServer := newHTTP(t, unpublishTestOrigin)
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
	repo := productRepo{db}
	publish := func(t *testing.T) string {
		t.Helper()
		ctx := t.Context()
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
	id := publish(t)
	proposal := map[string]string{"request_id": requestID, "product_id": id, "reason": "<script>immutable reason</script>"}
	statusInput := map[string]string{"request_id": requestID}
	httpServer.agent("/propose", "invalid", proposal, http.StatusUnauthorized)
	pending := httpServer.agent("/propose", token, proposal, http.StatusOK)
	if pending.Status != unpublishPending || pending.ReviewURL != unpublishTestOrigin+path {
		t.Fatalf("unexpected proposal status: %+v", pending)
	}
	var reviewPage struct {
		Props struct {
			Fields []struct{ Label, Value string }
		}
	}
	view := httpServer.request("GET", path, nil, jar, http.Header{"X-Inertia": {"true"}}, http.StatusOK)
	if err := json.Unmarshal(view, &reviewPage); err != nil {
		t.Fatal(err)
	}
	reviewFields := map[string]string{}
	for _, f := range reviewPage.Props.Fields {
		reviewFields[f.Label] = f.Value
	}
	if reviewFields["Товар"] != "<Review product>" || reviewFields["Причина"] != "<script>immutable reason</script>" || reviewFields["Переход"] != "Опубликован → Черновик" {
		t.Fatalf("unexpected immutable fields: %v", reviewFields)
	}
	httpServer.request("GET", path, nil, other, nil, http.StatusNotFound)
	approval := map[string]string{"decision": "approve"}
	httpServer.request("POST", path, approval, other, httpServer.formHeaders(other), http.StatusNotFound)
	missingOrigin := httpServer.formHeaders(jar)
	missingOrigin.Del("Origin")
	missingOrigin.Set("Referer", unpublishTestOrigin+path)
	httpServer.request("POST", path, approval, jar, missingOrigin, http.StatusForbidden)
	httpServer.request("POST", path, approval, jar, http.Header{"Origin": {unpublishTestOrigin}}, http.StatusFound)
	if got := httpServer.agent("/status", token, statusInput, http.StatusOK); got.Status != unpublishPending {
		t.Fatalf("failed attempts or GET changed request to %q", got.Status)
	}
	for range 2 {
		httpServer.request("POST", path, approval, jar, httpServer.formHeaders(jar), http.StatusOK)
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
	httpServer.agent("/propose", token, map[string]string{"request_id": pendingAfterRestart["request_id"], "product_id": publish(t), "reason": "restart"}, http.StatusOK)
	if err := httpServer.runtime.Close(); err != nil {
		t.Fatal(err)
	}
	httpServer = newHTTP(t, unpublishTestOrigin)
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
	checkPreview := func(s unpublishTestHTTP, jar unpublishTestCookies, token, requestID string) string {
		s.t.Helper()
		ctx := s.t.Context()
		id := publish(s.t)
		before, err := repo.get(ctx, actor, id)
		if err != nil {
			s.t.Fatal(err)
		}
		input := map[string]string{"request_id": requestID}
		proposal := map[string]string{"request_id": requestID, "product_id": id, "reason": "preview"}
		if got := s.agent("/propose", token, proposal, http.StatusOK); got.Status != unpublishPending || got.ReviewURL != s.origin+unpublishRequestPath+requestID {
			s.t.Fatalf("expected a pending proposal for the configured origin: %+v", got)
		}
		headers := s.formHeaders(jar)
		headers.Set("Accept", "application/json")
		s.request("POST", "/auth/role-preview", map[string]int64{"roleId": -1}, jar, headers, http.StatusBadRequest)
		if got := s.agent("/status", token, input, http.StatusOK); got.Status != unpublishPending {
			s.t.Fatalf("failed preview transition changed the pending review: %+v", got)
		}
		s.request("POST", "/auth/role-preview", map[string]int64{"roleId": previewCandidate.ID}, jar, s.formHeaders(jar), http.StatusOK)
		// Only the native transitions may retire the grant between these requests.
		s.request("POST", "/auth/role-preview", map[string]int64{"roleId": 0}, jar, s.formHeaders(jar), http.StatusOK)
		s.agent("/status", token, input, http.StatusUnauthorized)
		freshToken := s.pair(jar)
		if got := s.agent("/status", freshToken, input, http.StatusOK); got.Status != unpublishExpired {
			s.t.Fatalf("preview did not retire the pending review: %+v", got)
		}
		after, err := repo.get(ctx, actor, id)
		if err != nil || after.Revision != before.Revision || after.Data != before.Data || after.Data.Status != "published" {
			s.t.Fatalf("role preview changed the published product: %+v (%v)", after, err)
		}
		return freshToken
	}
	token = checkPreview(httpServer, jar, token, "d1f23610-53d6-4262-bb60-c343b578c03c")
	t.Run("production_role_preview", func(t *testing.T) {
		server := httptest.NewUnstartedServer(nil)
		defer server.Close()
		production := newHTTP(t, "https://"+server.Listener.Addr().String())
		server.Config.Handler = adaptor.FiberApp(production.runtime.App())
		server.StartTLS()
		client := server.Client()
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		production.do = client.Do
		jar := production.login(email)
		session := jar["__Host-"+unpublishTestSessionName]
		if session == nil || session.Value == "" || !session.Secure || !session.HttpOnly ||
			session.Domain != "" || session.Path != "/" || session.SameSite != http.SameSiteLaxMode {
			t.Fatal("production login did not issue a Secure, HttpOnly, host-only session cookie with Path=/ and SameSite=Lax")
		}
		if _, ok := jar[unpublishTestSessionName]; ok {
			t.Fatal("production login issued an unprefixed session cookie")
		}
		otherLogin := production.pair(production.login(email))
		current := checkPreview(production, jar, production.pair(jar), "d1f23610-53d6-4262-bb60-c343b578c07c")
		if got := production.agent("/status", otherLogin, statusInput, http.StatusOK); got != receipt {
			t.Fatalf("role preview affected another login: %+v", got)
		}

		// In this disposable database a foreign key makes deletion of this one
		// login fail. The native logout handler redirects on both outcomes.
		if _, err := db.ExecContext(t.Context(), `CREATE TABLE test_keep_login (id text REFERENCES goadmin_browser_auth_state(id))`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), `INSERT INTO test_keep_login VALUES ($1)`, session.Value); err != nil {
			t.Fatal(err)
		}
		logout := func() {
			t.Helper()
			headers := production.formHeaders(jar)
			headers.Set("Referer", production.origin+"/auth/profile")
			production.request(http.MethodDelete, "/auth/logout", nil, jar, headers, http.StatusFound)
		}
		logout()
		if got := production.agent("/status", current, statusInput, http.StatusOK); got != receipt {
			t.Fatalf("failed logout revoked the connection: %+v", got)
		}
		if _, err := db.ExecContext(t.Context(), `DROP TABLE test_keep_login`); err != nil {
			t.Fatal(err)
		}
		logout()
		if jar[session.Name] != nil {
			t.Fatal("successful logout did not expire the native session cookie")
		}
		production.agent("/status", current, statusInput, http.StatusUnauthorized)
	})

	t.Run("absolute browser expiry while the SQL row is live", func(t *testing.T) {
		live := httpServer.login(email)
		grant := httpServer.pair(live)
		key := live[unpublishTestSessionName].Value
		var raw []byte
		if err := db.QueryRowContext(ctx, "SELECT payload FROM goadmin_browser_sessions WHERE id=$1", key).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var values map[any]any
		if err := gob.NewDecoder(bytes.NewReader(raw)).Decode(&values); err != nil {
			t.Fatal(err)
		}
		found := false
		for k := range values {
			if reflect.TypeOf(k).Name() == "absExpirationKeyType" {
				values[k] = time.Now().Add(-time.Minute)
				found = true
			}
		}
		if !found {
			t.Fatal("fixture has no Fiber absolute deadline")
		}
		var encoded bytes.Buffer
		if err := gob.NewEncoder(&encoded).Encode(values); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, "UPDATE goadmin_browser_sessions SET payload=$1 WHERE id=$2", encoded.Bytes(), key); err != nil {
			t.Fatal(err)
		}
		httpServer.agent("/status", grant, statusInput, http.StatusUnauthorized)
		var retained bool
		if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM goadmin_browser_sessions WHERE id=$1)", key).Scan(&retained); err != nil || !retained {
			t.Fatalf("delegated observation modified the native session: %t %v", retained, err)
		}
	})

	// Share native state with a second runtime, so the first runtime's
	// transition observer cannot know about this preview until it resolves it.
	peer := newHTTP(t, unpublishTestOrigin)
	peer.request("POST", "/auth/role-preview", map[string]int64{"roleId": owner.ID}, jar, peer.formHeaders(jar), http.StatusOK)
	httpServer.agent("/status", token, statusInput, http.StatusForbidden)
	peer.request("POST", "/auth/role-preview", map[string]int64{"roleId": 0}, jar, peer.formHeaders(jar), http.StatusOK)
	token = httpServer.pair(jar)

	pendingPermission := map[string]string{"request_id": "d1f23610-53d6-4262-bb60-c343b578c04c"}
	httpServer.agent("/propose", token, map[string]string{"request_id": pendingPermission["request_id"], "product_id": publish(t), "reason": "permission"}, http.StatusOK)
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
	httpServer.agent("/propose", token, map[string]string{"request_id": browserDenied, "product_id": publish(t), "reason": "browser lost permission"}, http.StatusOK)
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
	blockedProduct := publish(t)
	httpServer.agent("/propose", token, map[string]string{"request_id": blockedID, "product_id": blockedProduct, "reason": "revoked"}, http.StatusOK)
	connectPage := httpServer.request("GET", unpublishConnectPath, nil, jar, http.Header{"X-Inertia": {"true"}}, http.StatusOK)
	var connections struct{ Props struct{ Grants []string } }
	if err := json.Unmarshal(connectPage, &connections); err != nil || len(connections.Props.Grants) != 1 {
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
	review.Header = httpServer.formHeaders(jar)
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
		resp, err := httpServer.runtime.App().Test(review, fiber.TestConfig{Timeout: 15 * time.Second})
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
	httpServer.request("POST", unpublishConnectPath, map[string]string{"decision": "revoke", "connection_id": connections.Props.Grants[0]},
		jar, httpServer.formHeaders(jar), http.StatusOK)
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
