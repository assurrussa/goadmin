package sso //nolint:testpackage // Handler boundary regressions.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	browser "github.com/assurrussa/gossoclient"
	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/host"
)

const (
	testNativeSession = "native-session"
	testOrigin        = "https://seo.example"
	testLinkURL       = "https://seo.example/sso-link"
)

type memoryIntents struct {
	secret, native string
	binding        host.ExternalBinding
	consumed       bool
	consumeCalls   int
}

func (m *memoryIntents) create(_ context.Context, secret, native string, b host.ExternalBinding) error {
	m.secret, m.native, m.binding, m.consumed = secret, native, b, false
	return nil
}

func (m *memoryIntents) consume(_ context.Context, secret, native string, local goauth.SubjectID) (host.ExternalBinding, error) {
	m.consumeCalls++
	if m.consumed || secret != m.secret || native != m.native || local != m.binding.LocalSubject {
		return host.ExternalBinding{}, sql.ErrNoRows
	}
	m.consumed = true
	return m.binding, nil
}

type fakeLinkAdmin struct {
	actor  host.Actor
	native host.CSRFRequest
	err    error
	linked []host.ExternalBinding
}

func (a *fakeLinkAdmin) CurrentActor(_ context.Context) (host.Actor, bool) {
	return a.actor, !a.actor.IsZero()
}

func (a *fakeLinkAdmin) AdminCSRFRequest(_ fiber.Ctx) (host.CSRFRequest, error) {
	return a.native, a.err
}

func (a *fakeLinkAdmin) LinkExternalAdminIdentity(_ fiber.Ctx, b host.ExternalBinding) (goauth.IdentityLink, error) {
	a.linked = append(a.linked, b)
	return goauth.IdentityLink{}, a.err
}

func linkFixture(t *testing.T) (*fiber.App, *fakeLinkAdmin, *memoryIntents, *fakeProtocol, string) {
	t.Helper()
	b, id, local, oldNow := snapshotFixture()
	delta := time.Since(oldNow)
	b.Deadline = b.Deadline.Add(delta)
	b.AbsoluteUntil = b.AbsoluteUntil.Add(delta)
	b.AuthTime = time.Now().Add(-time.Minute).Unix()
	id.IssuedAt = time.Now().Add(-time.Second)
	id.SessionExpiresAt = time.Now().Add(time.Hour)
	id.AbsoluteUntil = b.AbsoluteUntil
	id.AuthTime = b.AuthTime
	p := &fakeProtocol{id: id, binding: b}
	s := &Runtime{
		Config:     Config{Origin: testOrigin, Issuer: id.Issuer, ClientID: id.ClientID, ProjectID: id.ProjectID},
		csrfCookie: "csrf_token",
	}
	bound, err := buildSnapshot(b, id, local, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	p.binding.Deadline = bound.Deadline
	store := &memoryIntents{
		secret:  strings.Repeat("a", 43),
		native:  testNativeSession,
		binding: bound,
	}
	s.intents = store
	admin := &fakeLinkAdmin{
		actor:  host.Actor{AdminID: 1, SubjectID: local.String()},
		native: host.CSRFRequest{SessionID: store.native},
	}
	csrf, err := host.NewCSRFService(host.CSRFConfig{
		AppDomain:      "seo.example",
		SecretKey:      strings.Repeat("x", 32),
		AllowedOrigins: []string{s.Config.Origin},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	token, err := csrf.Create(t.Context(), admin.native)
	if err != nil {
		t.Fatal(err)
	}
	h := &linkHandler{
		service: s, admin: admin, csrf: csrf, verify: p.verify,
		takeReturn: func(context.Context, browser.Binding) (string, error) { return "/", nil },
		render:     func(c fiber.Ctx, _ string, props map[string]any) error { return c.JSON(props) },
	}
	app := fiber.New()
	// Handler tests target its own security checks. Core authentication, native
	// double-submit CSRF and role-preview denial need the assembled host gate.
	h.RegisterGroupRoutes(app)
	return app, admin, store, p, token.Token
}

func linkRequest(t *testing.T, app *fiber.App, secret, csrf, origin string) *http.Response {
	t.Helper()
	form := url.Values{"intent": {secret}, "csrf": {csrf}, "confirm": {"link-this-identity"}}
	r := httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, testLinkURL, strings.NewReader(form.Encode()),
	)
	r.Header.Set("Origin", origin)
	r.Header.Set("Accept", "application/json")
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := app.Test(r)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func TestExplicitBrowserLinkConsumesOnceWithoutNativeAdmission(t *testing.T) {
	app, admin, store, _, csrf := linkFixture(t)
	res1 := linkRequest(t, app, store.secret, csrf, testOrigin)
	defer res1.Body.Close()
	if res1.StatusCode != http.StatusOK || len(admin.linked) != 1 || !store.consumed {
		t.Fatalf("link failed: status %d", res1.StatusCode)
	}
	if admin.linked[0] != store.binding {
		t.Fatal("link identity changed after confirmation")
	}
	res2 := linkRequest(t, app, store.secret, csrf, testOrigin)
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusForbidden || len(admin.linked) != 1 {
		t.Fatal("confirmation replay linked twice")
	}
}

func TestBrowserLinkDeniesWrongNativeSessionCSRFOriginOrRP(t *testing.T) {
	cases := []struct {
		name string
		edit func(*fakeLinkAdmin, *memoryIntents, *fakeProtocol, *string, *string)
	}{
		{"native replaced", func(a *fakeLinkAdmin, _ *memoryIntents, _ *fakeProtocol, _ *string, _ *string) {
			a.native.SessionID = "other-session"
		}},
		{"canonical subject changed", func(a *fakeLinkAdmin, _ *memoryIntents, _ *fakeProtocol, _ *string, _ *string) {
			a.actor.SubjectID = goauth.NewSubjectID().String()
		}},
		{"wrong CSRF", func(_ *fakeLinkAdmin, _ *memoryIntents, _ *fakeProtocol, csrf *string, _ *string) { *csrf = "bad" }},
		{"wrong origin", func(_ *fakeLinkAdmin, _ *memoryIntents, _ *fakeProtocol, _ *string, origin *string) {
			*origin = "https://attacker.example"
		}},
		{"RP replaced", func(_ *fakeLinkAdmin, _ *memoryIntents, p *fakeProtocol, _ *string, _ *string) {
			p.binding.LoginGeneration++
		}},
		{"RP outage", func(_ *fakeLinkAdmin, _ *memoryIntents, p *fakeProtocol, _ *string, _ *string) {
			p.err = errors.New("unavailable")
		}},
		{"native not fresh", func(a *fakeLinkAdmin, _ *memoryIntents, _ *fakeProtocol, _ *string, _ *string) {
			a.err = errors.New("native invalid")
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			app, admin, store, p, csrf := linkFixture(t)
			origin := testOrigin
			test.edit(admin, store, p, &csrf, &origin)
			res := linkRequest(t, app, store.secret, csrf, origin)
			defer res.Body.Close()
			if res.StatusCode != http.StatusForbidden || len(admin.linked) != 0 {
				t.Fatalf("invalid binding linked: status %d", res.StatusCode)
			}
		})
	}
}

type fakeProtocol struct {
	id      browser.Identity
	binding browser.Binding
	err     error
}

func (p *fakeProtocol) verify(*http.Request) (browser.Identity, browser.Binding, error) {
	return p.id, p.binding, p.err
}

func TestLinkPagePassesVerifiedIdentityAndNativeBoundIntentToRenderer(t *testing.T) {
	app, admin, store, protocol, _ := linkFixture(t)
	protocol.id.Subject = `<script>alert(1)</script>`
	protocol.binding.Subject = protocol.id.Subject
	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, testLinkURL, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status %d", response.StatusCode)
	}
	var props struct {
		Subject string `json:"subject"`
		Intent  string `json:"intent"`
		CSRF    string `json:"csrf"`
	}
	if err = json.NewDecoder(response.Body).Decode(&props); err != nil {
		t.Fatal(err)
	}
	if props.Subject != protocol.id.Subject || len(props.Intent) != 43 || props.CSRF == "" {
		t.Fatal("lost verified page props")
	}
	if store.binding.LocalSubject.String() != admin.actor.SubjectID ||
		store.native != admin.native.SessionID || store.secret != props.Intent {
		t.Fatal("intent lost native binding")
	}
	cookies := response.Cookies()
	if len(cookies) != 1 || cookies[0].Value != props.CSRF || cookies[0].Name != "csrf_token" {
		t.Fatal("native CSRF cookie mismatch")
	}
}

func TestLinkReportsProviderUnavailableWithoutRetryingIntent(t *testing.T) {
	app, admin, store, protocol, csrf := linkFixture(t)
	protocol.err = browser.ErrUnavailable
	response := linkRequest(t, app, store.secret, csrf, testOrigin)
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable || len(admin.linked) != 0 || !store.consumed {
		t.Fatal("unavailable proof must fail closed with a consumed intent and retry-later status")
	}
}
