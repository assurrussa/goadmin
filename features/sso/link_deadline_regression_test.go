package sso //nolint:testpackage // Exercises the internal link boundary and fixture.

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	browser "github.com/assurrussa/gossoclient"

	"github.com/assurrussa/goadmin/host"
)

const (
	testGenerationReason = "generation"
	testSubjectReason    = "subject"
	testDifferentValue   = "different"
)

// The route reports a fixed reason without exposing either binding value.
func TestLinkReportsDeadlineMismatch(t *testing.T) {
	app, admin, store, protocol, csrf := linkFixture(t)
	protocol.binding.Deadline = time.Now().Add(time.Hour)
	if !protocol.binding.Deadline.After(store.binding.Deadline) {
		t.Fatal("fixture must have a later SDK deadline than capped snapshot")
	}
	data, err := json.Marshal(store.binding)
	if err != nil {
		t.Fatal(err)
	}
	var persisted host.ExternalBinding
	if err = json.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	store.binding = persisted
	response := linkRequest(t, app, store.secret, csrf, testOrigin)
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden || len(admin.linked) != 0 {
		t.Fatalf("binding mismatch not rejected: status=%d, links=%d", response.StatusCode, len(admin.linked))
	}
	assertLinkCodeReason(t, response, "link_binding_changed", "deadline")
}

func TestLinkAcceptsEquivalentTimesWithDifferentLocations(t *testing.T) {
	app, admin, store, protocol, csrf := linkFixture(t)
	data, err := json.Marshal(store.binding)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &store.binding); err != nil {
		t.Fatal(err)
	}
	zone := time.FixedZone("plus-three", 3*60*60)
	protocol.binding.Deadline = protocol.binding.Deadline.In(zone)
	protocol.binding.AbsoluteUntil = protocol.binding.AbsoluteUntil.In(zone)
	pinned, err := sdkBinding(store.binding)
	if err != nil || protocol.binding == pinned || sdkBindingDifference(protocol.binding, pinned) != "" {
		t.Fatalf("fixture did not isolate representational time mismatch: %v", err)
	}
	response := linkRequest(t, app, store.secret, csrf, testOrigin)
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || len(admin.linked) != 1 {
		t.Fatalf("equivalent binding rejected: status=%d, links=%d", response.StatusCode, len(admin.linked))
	}
}

func TestLinkReportsBareDenialStages(t *testing.T) {
	for _, change := range []struct {
		name   string
		edit   func(*fakeLinkAdmin, *fakeProtocol)
		origin string
		code   string
		reason string
	}{
		{
			testGenerationReason, func(_ *fakeLinkAdmin, p *fakeProtocol) { p.binding.Generation++ }, testOrigin,
			"link_binding_changed", testGenerationReason,
		},
		{"identity", func(_ *fakeLinkAdmin, p *fakeProtocol) {
			p.id.Subject = "different-subject"
			p.binding.Subject = p.id.Subject
		}, testOrigin, "link_binding_changed", testSubjectReason},
		{"native session", func(a *fakeLinkAdmin, _ *fakeProtocol) {
			a.err = errors.New("unavailable")
		}, testOrigin, "link_native_session_invalid", ""},
		{"wrong origin", func(_ *fakeLinkAdmin, _ *fakeProtocol) {}, "https://other.example", "link_request_invalid", ""},
	} {
		t.Run(change.name, func(t *testing.T) {
			app, admin, store, protocol, csrf := linkFixture(t)
			change.edit(admin, protocol)
			response := linkRequest(t, app, store.secret, csrf, change.origin)
			defer response.Body.Close()
			if response.StatusCode != http.StatusForbidden || len(admin.linked) != 0 {
				t.Fatalf("changed binding accepted: status=%d, links=%d", response.StatusCode, len(admin.linked))
			}
			assertLinkCodeReason(t, response, change.code, change.reason)
		})
	}
}

func TestLinkReportsInvalidStoredBinding(t *testing.T) {
	app, admin, store, _, csrf := linkFixture(t)
	store.binding.Proof.IssuedAt = store.binding.Deadline.Add(time.Second)
	response := linkRequest(t, app, store.secret, csrf, testOrigin)
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden || len(admin.linked) != 0 {
		t.Fatalf("invalid stored binding accepted: status=%d, links=%d", response.StatusCode, len(admin.linked))
	}
	assertLinkCodeReason(t, response, "link_binding_changed", "stored_binding_invalid")
}

func TestSDKBindingDifferenceCoversEveryPinnedField(t *testing.T) {
	base, id, local, now := snapshotFixture()
	if id.Subject != base.Subject || local.IsZero() || now.IsZero() {
		t.Fatal("invalid base binding fixture")
	}
	for _, change := range []struct {
		reason string
		edit   func(*browser.Binding)
	}{
		{"reference", func(b *browser.Binding) { b.Reference = testDifferentValue }},
		{testGenerationReason, func(b *browser.Binding) { b.Generation++ }},
		{"login_generation", func(b *browser.Binding) { b.LoginGeneration++ }},
		{"deadline", func(b *browser.Binding) { b.Deadline = b.Deadline.Add(time.Second) }},
		{"issuer", func(b *browser.Binding) { b.Issuer = "https://other.test" }},
		{testSubjectReason, func(b *browser.Binding) { b.Subject = testDifferentValue }},
		{"project_id", func(b *browser.Binding) { b.ProjectID = testDifferentValue }},
		{"client_id", func(b *browser.Binding) { b.ClientID = testDifferentValue }},
		{"central_session_id", func(b *browser.Binding) { b.CentralSessionID = testDifferentValue }},
		{"auth_time", func(b *browser.Binding) { b.AuthTime++ }},
		{"absolute_until", func(b *browser.Binding) { b.AbsoluteUntil = b.AbsoluteUntil.Add(time.Second) }},
	} {
		t.Run(change.reason, func(t *testing.T) {
			changed := base
			change.edit(&changed)
			if got := sdkBindingDifference(changed, base); got != change.reason {
				t.Fatalf("difference=%q; want %q", got, change.reason)
			}
		})
	}
}

func assertLinkCodeReason(t *testing.T, response *http.Response, expectedCode, expectedReason string) {
	t.Helper()
	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Reason  string `json:"reason"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("missing structured failure: %v", err)
	}
	if body.Code != expectedCode || body.Message == "" || body.Reason != expectedReason {
		t.Fatalf("code=%q, reason=%q, message present=%t; want %q/%q",
			body.Code, body.Reason, body.Message != "", expectedCode, expectedReason)
	}
}
