package sso //nolint:testpackage // Canonical admission boundary regression.

import (
	"context"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	browser "github.com/assurrussa/gossoclient"
)

type fixtureLinks struct {
	account goauth.Account
	link    goauth.IdentityLink
	err     error
}

func (f fixtureLinks) ResolveIdentityLink(context.Context, string, string) (goauth.Account, goauth.IdentityLink, error) {
	return f.account, f.link, f.err
}

func TestCanonicalResolutionNeverUsesEmailOrInactiveIdentity(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*Config, *browser.Identity, *browser.Binding, *fixtureLinks)
	}{
		{"missing canonical link", func(_ *Config, _ *browser.Identity, _ *browser.Binding, f *fixtureLinks) {
			f.err = goauth.ErrIdentityLinkNotFound
		}},
		{"inactive subject", func(_ *Config, _ *browser.Identity, _ *browser.Binding, f *fixtureLinks) {
			f.account.Subject.Status = "disabled"
		}},
		{"wrong local subject", func(_ *Config, _ *browser.Identity, _ *browser.Binding, f *fixtureLinks) {
			f.link.SubjectID = goauth.NewSubjectID()
		}},
		{"wrong linked issuer", func(_ *Config, _ *browser.Identity, _ *browser.Binding, f *fixtureLinks) {
			f.link.Issuer = "https://other.invalid"
		}},
		{"wrong linked subject", func(_ *Config, _ *browser.Identity, _ *browser.Binding, f *fixtureLinks) {
			f.link.ExternalSubject = testOther
		}},
		{"wrong client", func(c *Config, _ *browser.Identity, _ *browser.Binding, _ *fixtureLinks) { c.ClientID = testOther }},
		{"wrong project", func(c *Config, _ *browser.Identity, _ *browser.Binding, _ *fixtureLinks) { c.ProjectID = testOther }},
		{"wrong generation", func(_ *Config, _ *browser.Identity, b *browser.Binding, _ *fixtureLinks) {
			b.LoginGeneration = b.Generation + 1
		}},
		{"expired proof", func(_ *Config, _ *browser.Identity, b *browser.Binding, _ *fixtureLinks) {
			b.Deadline = time.Now().Add(-time.Second)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, id, local, _ := snapshotFixture()
			now := time.Now()
			b.AuthTime = now.Add(-time.Minute).Unix()
			b.Deadline = now.Add(time.Minute)
			b.AbsoluteUntil = now.Add(time.Hour)
			id.AuthTime = b.AuthTime
			id.IssuedAt = now.Add(-time.Second)
			id.SessionExpiresAt = now.Add(time.Hour)
			id.AbsoluteUntil = b.AbsoluteUntil
			c := Config{Issuer: id.Issuer, ClientID: id.ClientID, ProjectID: id.ProjectID}
			f := fixtureLinks{
				account: goauth.Account{Subject: goauth.Subject{ID: local, Status: goauth.SubjectStatusActive}},
				link:    goauth.IdentityLink{ID: "link", SubjectID: local, Issuer: id.Issuer, ExternalSubject: id.Subject},
			}
			if _, err := (&Runtime{Config: c}).resolveVerified(t.Context(), id, b, f); err != nil {
				t.Fatal("valid control rejected", err)
			}
			tc.edit(&c, &id, &b, &f)
			if _, err := (&Runtime{Config: c}).resolveVerified(t.Context(), id, b, f); err == nil {
				t.Fatal("invalid canonical admission accepted")
			}
		})
	}
}
