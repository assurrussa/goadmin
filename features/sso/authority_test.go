package sso //nolint:testpackage // Tests exercise internal binding and recovery invariants.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	browser "github.com/assurrussa/gossoclient"

	"github.com/assurrussa/goadmin/host"
)

func snapshotFixture() (browser.Binding, browser.Identity, goauth.SubjectID, time.Time) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	b := browser.Binding{
		Reference:        "opaque-reference",
		Generation:       2,
		LoginGeneration:  1,
		Deadline:         now.Add(time.Hour),
		Issuer:           "https://auth.example.test",
		Subject:          "external-subject",
		ProjectID:        "project",
		ClientID:         "client",
		CentralSessionID: "central-session",
		AuthTime:         now.Add(-time.Minute).Unix(),
		AbsoluteUntil:    now.Add(24 * time.Hour),
	}
	id := browser.Identity{
		Issuer:           b.Issuer,
		Subject:          b.Subject,
		ProjectID:        b.ProjectID,
		ClientID:         b.ClientID,
		SessionID:        b.CentralSessionID,
		AuthTime:         b.AuthTime,
		IssuedAt:         now.Add(-time.Second),
		SessionExpiresAt: now.Add(time.Hour),
		AbsoluteUntil:    b.AbsoluteUntil,
	}
	return b, id, goauth.NewSubjectID(), now
}

func TestSnapshotPinsAndFreshness(t *testing.T) {
	b, id, local, now := snapshotFixture()
	got, err := buildSnapshot(b, id, local, "link", now)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Deadline.Equal(id.IssuedAt.Add(freshnessCap)) {
		t.Fatal("freshness cap lost")
	}
	if got.LocalSubject != local || got.LinkID != "link" || got.LoginGeneration != 1 || got.Generation != 2 {
		t.Fatal("identity pins lost")
	}
	roundtrip, err := sdkBinding(got)
	if err != nil || roundtrip.Reference != b.Reference || !roundtrip.Deadline.Equal(got.Deadline) {
		t.Fatalf("binding: %v", err)
	}
}

func TestSnapshotRejectsInconsistentProof(t *testing.T) {
	cases := map[string]func(*browser.Binding, *browser.Identity){
		"issuer":       func(_ *browser.Binding, i *browser.Identity) { i.Issuer = "https://other.test" },
		"subject":      func(_ *browser.Binding, i *browser.Identity) { i.Subject = testOther },
		"project":      func(_ *browser.Binding, i *browser.Identity) { i.ProjectID = testOther },
		"client":       func(_ *browser.Binding, i *browser.Identity) { i.ClientID = testOther },
		"session":      func(_ *browser.Binding, i *browser.Identity) { i.SessionID = testOther },
		"generation":   func(b *browser.Binding, _ *browser.Identity) { b.Generation = 0 },
		"lineage":      func(b *browser.Binding, _ *browser.Identity) { b.LoginGeneration = b.Generation + 1 },
		"future proof": func(_ *browser.Binding, i *browser.Identity) { i.IssuedAt = i.IssuedAt.Add(time.Hour) },
		"expired":      func(b *browser.Binding, _ *browser.Identity) { b.Deadline = b.Deadline.Add(-2 * time.Hour) },
		"absolute":     func(_ *browser.Binding, i *browser.Identity) { i.AbsoluteUntil = i.AbsoluteUntil.Add(time.Hour) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			b, id, local, now := snapshotFixture()
			change(&b, &id)
			if _, err := buildSnapshot(b, id, local, "link", now); err == nil {
				t.Fatal("invalid snapshot accepted")
			}
		})
	}
}

type fakeLifecycle struct {
	checks, revokes int
	revoked         browser.Binding
}

func (f *fakeLifecycle) Check(context.Context, browser.Binding) (browser.Proof, error) {
	f.checks++
	return browser.Proof{}, errors.New("unavailable")
}

func (f *fakeLifecycle) RevokeBinding(_ context.Context, b browser.Binding) error {
	f.revokes++
	f.revoked = b
	return nil
}

func TestLogoutPreservesExpiredLineageAndDetachDoesNotRevoke(t *testing.T) {
	b, id, local, now := snapshotFixture()
	binding, err := buildSnapshot(b, id, local, "link", now)
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeLifecycle{}
	a := &Authority{Client: client, Allows: func(string, string) bool { return false }}
	if err := a.Detach(context.Background(), binding); err != nil || client.revokes != 0 {
		t.Fatal("detach revoked authority")
	}
	if err := a.Logout(context.Background(), binding); err != nil {
		t.Fatal(err)
	}
	if client.revokes != 1 || client.checks != 0 || client.revoked.LoginGeneration != 1 {
		t.Fatal("logout lost expired original lineage")
	}
}

func TestInvalidAndDisallowedBindingsNeverReachProvider(t *testing.T) {
	client := &fakeLifecycle{}
	a := &Authority{Client: client, Allows: func(string, string) bool { return false }}
	if _, err := a.Validate(context.Background(), host.ExternalBinding{}); err == nil {
		t.Fatal("zero binding accepted")
	}
	if client.checks != 0 {
		t.Fatal("invalid binding reached provider")
	}
}
