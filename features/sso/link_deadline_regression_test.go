package sso //nolint:testpackage // Exercises the internal link boundary and fixture.

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/assurrussa/goadmin/host"
)

// The route must identify a binding mismatch without exposing binding values.
func TestLinkReportsBindingMismatch(t *testing.T) {
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
	assertLinkCode(t, response, "link_binding_changed")
}

func TestLinkReportsBareDenialStages(t *testing.T) {
	for _, change := range []struct {
		name   string
		edit   func(*fakeLinkAdmin, *fakeProtocol)
		origin string
		code   string
	}{
		{"generation", func(_ *fakeLinkAdmin, p *fakeProtocol) { p.binding.Generation++ }, testOrigin, "link_binding_changed"},
		{"identity", func(_ *fakeLinkAdmin, p *fakeProtocol) {
			p.id.Subject = "different-subject"
			p.binding.Subject = p.id.Subject
		}, testOrigin, "link_binding_changed"},
		{"native session", func(a *fakeLinkAdmin, _ *fakeProtocol) {
			a.err = errors.New("unavailable")
		}, testOrigin, "link_native_session_invalid"},
		{"wrong origin", func(_ *fakeLinkAdmin, _ *fakeProtocol) {}, "https://other.example", "link_request_invalid"},
	} {
		t.Run(change.name, func(t *testing.T) {
			app, admin, store, protocol, csrf := linkFixture(t)
			change.edit(admin, protocol)
			response := linkRequest(t, app, store.secret, csrf, change.origin)
			defer response.Body.Close()
			if response.StatusCode != http.StatusForbidden || len(admin.linked) != 0 {
				t.Fatalf("changed binding accepted: status=%d, links=%d", response.StatusCode, len(admin.linked))
			}
			assertLinkCode(t, response, change.code)
		})
	}
}

func assertLinkCode(t *testing.T, response *http.Response, expected string) {
	t.Helper()
	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("missing structured failure: %v", err)
	}
	if body.Code != expected || body.Message == "" {
		t.Fatalf("code=%q, message present=%t; want %q", body.Code, body.Message != "", expected)
	}
}
