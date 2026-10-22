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
	testNativeSession    = "native-session"
	testOrigin           = "https://seo.example"
	testLinkURL          = "https://seo.example/sso-link"
	testLinkConfirmField = "confirm"
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
	actor       host.Actor
	native      host.CSRFRequest
	err         error
	linkErr     error
	linked      []host.ExternalBinding
	eligibility host.IdentityLinkEligibility
	logouts     int
	hasLink     bool
}

func (a *fakeLinkAdmin) LocalIdentityLinkEligibility(fiber.Ctx) (host.IdentityLinkEligibility, error) {
	if a.eligibility != "" {
		return a.eligibility, nil
	}
	return host.IdentityLinkFresh, nil
}

func (a *fakeLinkAdmin) LogoutLocalAdminAuth(fiber.Ctx) error { a.logouts++; return nil }

func (a *fakeLinkAdmin) HasExternalAdminIdentity(fiber.Ctx, string, string) (bool, error) {
	return a.hasLink, nil
}

func (a *fakeLinkAdmin) CurrentActor(_ context.Context) (host.Actor, bool) {
	return a.actor, !a.actor.IsZero()
}

func (a *fakeLinkAdmin) AdminCSRFRequest(_ fiber.Ctx) (host.CSRFRequest, error) {
	return a.native, a.err
}

func (a *fakeLinkAdmin) LinkExternalAdminIdentity(_ fiber.Ctx, b host.ExternalBinding) (goauth.IdentityLink, error) {
	a.linked = append(a.linked, b)
	if a.linkErr != nil {
		return goauth.IdentityLink{}, a.linkErr
	}
	return goauth.IdentityLink{}, a.err
}

func linkFixture(t *testing.T, customCookie ...string) (*fiber.App, *fakeLinkAdmin, *memoryIntents, *fakeProtocol, string) {
	t.Helper()
	cookieName := testLinkCSRFCookie
	if len(customCookie) != 0 {
		cookieName = customCookie[0]
	}
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
		csrfCookie: cookieName,
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

func TestProviderAccountNavigationRequiresNativeActorAndUsesOnlyConfiguredTarget(t *testing.T) {
	_, fixtureAdmin, _, _, _ := linkFixture(t)
	admin := &fakeLinkAdmin{actor: fixtureAdmin.actor, native: fixtureAdmin.native}
	service := &Runtime{Config: testConfig()}
	h := &linkHandler{service: service, admin: admin}
	app := fiber.New()
	h.RegisterGroupRoutes(app)
	want := service.Config.Issuer + "/sso/account?client_id=" + service.Config.ClientID +
		"&redirect_uri=https%3A%2F%2Fadmin.example.test%2Fcallback"
	for _, tc := range []struct {
		name, path, inertia, authorization string
		actor                              bool
		status                             int
	}{
		{"document", providerAccountPath + "?redirect_uri=https://attacker.invalid", "", "", true, http.StatusSeeOther},
		{"inertia", providerAccountPath, "true", "", true, http.StatusConflict},
		{"no actor", providerAccountPath, "", "", false, http.StatusForbidden},
		{"bearer", providerAccountPath, "", "Bearer fabricated", true, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.actor {
				admin.actor = fixtureAdmin.actor
			} else {
				admin.actor = host.Actor{}
			}
			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, tc.path, nil)
			if tc.inertia != "" {
				request.Header.Set("X-Inertia", tc.inertia)
			}
			if tc.authorization != "" {
				request.Header.Set("Authorization", tc.authorization)
			}
			response, err := app.Test(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != tc.status {
				t.Fatalf("status: %d", response.StatusCode)
			}
			if tc.status == http.StatusSeeOther && response.Header.Get("Location") != want {
				t.Fatalf("target: %q", response.Header.Get("Location"))
			}
			if tc.status == http.StatusConflict && response.Header.Get("X-Inertia-Location") != want {
				t.Fatalf("inertia target: %q", response.Header.Get("X-Inertia-Location"))
			}
		})
	}
}

func linkRequest(t *testing.T, app *fiber.App, secret, csrf, origin string) *http.Response {
	t.Helper()
	form := url.Values{"intent": {secret}, testLinkCSRFField: {csrf}, testLinkConfirmField: {"link-this-identity"}}
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
	var props map[string]any
	if err = json.NewDecoder(response.Body).Decode(&props); err != nil {
		t.Fatal(err)
	}
	subject, _ := props["subject"].(string)
	intent, _ := props["intent"].(string)
	csrfProp, _ := props["csrf"].(string)
	if subject != protocol.id.Subject || len(intent) != 43 || csrfProp == "" || props[testLinkCSRFCookie] != csrfProp {
		t.Fatal("lost verified page props")
	}
	if store.binding.LocalSubject.String() != admin.actor.SubjectID ||
		store.native != admin.native.SessionID || store.secret != intent {
		t.Fatal("intent lost native binding")
	}
	cookies := response.Cookies()
	if len(cookies) != 1 || cookies[0].Value != csrfProp || cookies[0].Name != testLinkCSRFCookie {
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

func TestStaleLocalLinkPromptsBeforeIntentOrMutation(t *testing.T) {
	app, admin, store, _, csrf := linkFixture(t)
	admin.eligibility = host.IdentityLinkStale
	page, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, testLinkURL, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer page.Body.Close()
	var props map[string]any
	if err := json.NewDecoder(page.Body).Decode(&props); err != nil {
		t.Fatal(err)
	}
	if page.StatusCode != http.StatusOK || props["code"] != "local_auth_stale" || props["reauthCSRF"] == "" ||
		props[testLinkCSRFCookie] != props["reauthCSRF"] {
		t.Fatalf("stale GET did not offer explicit reauthentication: %v", props)
	}
	res := linkRequest(t, app, store.secret, csrf, testOrigin)
	defer res.Body.Close()
	var failure map[string]any
	if err := json.NewDecoder(res.Body).Decode(&failure); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusForbidden || failure["code"] != "local_auth_stale" ||
		store.consumeCalls != 0 || len(admin.linked) != 0 || admin.logouts != 0 {
		t.Fatalf("stale POST consumed intent, linked, or logged out: %v", failure)
	}
}

func TestStaleReauthenticationProofUsesConfiguredCookieName(t *testing.T) {
	const cookieName = "custom_sso_csrf"
	app, admin, intents, protocol, proof := linkFixture(t, cookieName)
	_ = intents
	_ = protocol
	_ = proof
	admin.eligibility = host.IdentityLinkStale
	page, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, testLinkURL, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer page.Body.Close()
	var props map[string]any
	if err := json.NewDecoder(page.Body).Decode(&props); err != nil {
		t.Fatal(err)
	}
	if page.StatusCode != http.StatusOK || props[cookieName] == "" || props[cookieName] != props["reauthCSRF"] ||
		len(page.Cookies()) != 1 || page.Cookies()[0].Name != cookieName || page.Cookies()[0].Value != props[cookieName] {
		t.Fatal("configured CSRF cookie did not match the stale form and shared prop")
	}
}

func TestExternalNativeSessionCannotEnterLinkLoop(t *testing.T) {
	app, admin, store, _, _ := linkFixture(t)
	admin.eligibility = host.IdentityLinkNotLocal
	response, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, testLinkURL, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var props map[string]any
	if err := json.NewDecoder(response.Body).Decode(&props); err != nil {
		t.Fatal(err)
	}
	linkAvailable, linkOK := props["linkAvailable"].(bool)
	startAvailable, startOK := props["startAvailable"].(bool)
	if response.StatusCode != http.StatusForbidden || props["code"] != "local_auth_required" ||
		!linkOK || linkAvailable || !startOK || startAvailable || store.consumeCalls != 0 {
		t.Fatalf("external native session offered linking: %v", props)
	}
}

func TestExplicitLocalReauthenticationRequiresCSRFAndNeverLinks(t *testing.T) {
	app, admin, store, _, csrf := linkFixture(t)
	admin.eligibility = host.IdentityLinkStale
	form := url.Values{"csrf": {csrf}, testLinkConfirmField: {"reauthenticate-local"}}
	request := func(origin, token string) (int, string) {
		form.Set("csrf", token)
		r := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
			testOrigin+"/sso-link/reauth", strings.NewReader(form.Encode()))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		res, err := app.Test(r)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		return res.StatusCode, res.Header.Get("Location")
	}
	if status, _ := request(testOrigin, "bad"); status != http.StatusForbidden || admin.logouts != 0 {
		t.Fatal("invalid CSRF triggered logout")
	}
	if status, _ := request("https://evil.invalid", csrf); status != http.StatusForbidden || admin.logouts != 0 {
		t.Fatal("wrong Origin triggered logout")
	}
	status, location := request(testOrigin, csrf)
	if status != http.StatusSeeOther || location != "/auth/login?local=1&redirectback=%2Fsso-link" ||
		admin.logouts != 1 || store.consumeCalls != 0 || len(admin.linked) != 0 {
		t.Fatal("explicit reauthentication did not return to a new form safely")
	}
}

func TestExplicitLocalReauthenticationJSONResponseIsFixedAndLocalOnly(t *testing.T) {
	app, admin, store, _, csrf := linkFixture(t)
	admin.eligibility = host.IdentityLinkStale
	form := url.Values{"csrf": {csrf}, testLinkConfirmField: {"reauthenticate-local"}}
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		testOrigin+"/sso-link/reauth", strings.NewReader(form.Encode()))
	r.Header.Set("Origin", testOrigin)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Accept", "application/json")
	res, err := app.Test(r)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body struct {
		Next string `json:"next"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK || body.Next != "/auth/login?local=1&redirectback=%2Fsso-link" ||
		admin.logouts != 1 || store.consumeCalls != 0 || len(admin.linked) != 0 {
		t.Fatalf("unexpected JSON reauthentication response: status=%d next=%q logouts=%d",
			res.StatusCode, body.Next, admin.logouts)
	}
}

func TestIdentityConflictIsDistinctFromStaleAndDependencyFailure(t *testing.T) {
	app, admin, store, _, csrf := linkFixture(t)
	admin.linkErr = goauth.ErrIdentityLinkConflict
	res := linkRequest(t, app, store.secret, csrf, testOrigin)
	defer res.Body.Close()
	var failure map[string]any
	if err := json.NewDecoder(res.Body).Decode(&failure); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusConflict || failure["code"] != "identity_link_conflict" || !store.consumed {
		t.Fatalf("conflict classification: %v", failure)
	}
}

func TestReadOnlyAccountStatusShowsPendingOnlyWithoutCanonicalLink(t *testing.T) {
	app, admin, store, provider, _ := linkFixture(t)
	for _, test := range []struct {
		linked, verified, mismatch bool
		pending                    bool
	}{
		{false, true, false, true},
		{true, true, false, false},
		{false, false, false, false},
		{false, true, true, false},
	} {
		admin.hasLink = test.linked
		provider.err = nil
		provider.id.Subject = provider.binding.Subject
		if !test.verified {
			provider.err = browser.ErrDenied
		}
		if test.mismatch {
			provider.id.Subject = "different-subject"
		}
		res, err := app.Test(httptest.NewRequestWithContext(t.Context(), http.MethodGet, testOrigin+"/sso-account", nil))
		if err != nil {
			t.Fatal(err)
		}
		var status struct {
			Linked    bool   `json:"linked"`
			Pending   bool   `json:"pending"`
			LocalAuth string `json:"localAuth"`
		}
		if err := json.NewDecoder(res.Body).Decode(&status); err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK || status.Linked != test.linked ||
			status.Pending != test.pending || status.LocalAuth != "fresh" || store.consumeCalls != 0 {
			t.Fatalf("account status: %+v, case %+v", status, test)
		}
	}
}
