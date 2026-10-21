package sso

import (
	"context"
	"net/http"
	"time"

	"github.com/assurrussa/goauth"
	browser "github.com/assurrussa/gossoclient"

	"github.com/assurrussa/goadmin/host"
)

var ErrDenied = browser.ErrDenied

type LinkResolver interface {
	ResolveIdentityLink(ctx context.Context, issuer, subject string) (goauth.Account, goauth.IdentityLink, error)
}

func (s *Runtime) verify(r *http.Request) (browser.Identity, browser.Binding, error) {
	if !s.Config.AllowsRequest(r) {
		return browser.Identity{}, browser.Binding{}, ErrDenied
	}
	proof, err := s.Client.Verify(r)
	if err != nil {
		return browser.Identity{}, browser.Binding{}, err
	}
	return proof.Identity(), proof.Binding(), nil
}

func (s *Runtime) matchesSnapshot(id browser.Identity, b browser.Binding) bool {
	return id.Issuer == s.Config.Issuer && id.ClientID == s.Config.ClientID &&
		(s.Config.ProjectID == "" || id.ProjectID == s.Config.ProjectID) &&
		id.Issuer == b.Issuer && id.ClientID == b.ClientID && id.Subject == b.Subject &&
		id.ProjectID == b.ProjectID
}

func (s *Runtime) resolve(ctx context.Context, proof browser.Proof, links LinkResolver) (host.ExternalBinding, error) {
	return s.resolveVerified(ctx, proof.Identity(), proof.Binding(), links)
}

// resolveVerified is internal; callers must first extract the SDK-verified proof.
func (s *Runtime) resolveVerified(
	ctx context.Context, id browser.Identity, b browser.Binding, links LinkResolver,
) (host.ExternalBinding, error) {
	if links == nil || !s.matchesSnapshot(id, b) {
		return host.ExternalBinding{}, ErrDenied
	}
	account, link, err := links.ResolveIdentityLink(ctx, id.Issuer, id.Subject)
	if err != nil {
		return host.ExternalBinding{}, err
	}
	if account.Subject.Status != goauth.SubjectStatusActive || link.SubjectID != account.Subject.ID ||
		link.ID == "" || link.Issuer != id.Issuer || link.ExternalSubject != id.Subject {
		return host.ExternalBinding{}, ErrDenied
	}
	return buildSnapshot(b, id, account.Subject.ID, link.ID, time.Now())
}
