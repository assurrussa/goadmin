package sso

import (
	"context"
	"errors"
	"math"
	"net/http"
	"time"

	"github.com/assurrussa/goauth"
	browser "github.com/assurrussa/gossoclient"

	"github.com/assurrussa/goadmin/host"
)

const freshnessCap = 5 * time.Minute

type Client interface {
	Check(ctx context.Context, binding browser.Binding) (browser.Proof, error)
	RevokeBinding(ctx context.Context, binding browser.Binding) error
}

type Authority struct {
	Client Client
	Allows func(issuer, subject string) bool
	// AllowsRequest opts into core maintenance and enforces the same ingress
	// restrictions as the optional browser adapter.
	AllowsRequest func(*http.Request) bool
}

var _ host.ExternalSessionAuthority = (*Authority)(nil)

var _ host.ExternalSessionMaintainer = (*Authority)(nil)

func (a *Authority) MaintenanceEnabled() bool {
	if a == nil || a.Allows == nil || a.AllowsRequest == nil {
		return false
	}
	_, ok := a.Client.(interface {
		Verify(request *http.Request) (browser.Proof, error)
	})
	return ok
}

// Renew uses the existing SDK owner protocol under GoAdmin's outer native
// owner. It does not weaken Validate or create a new admission.
func (a *Authority) Renew(r *http.Request, old host.ExternalBinding) (host.ExternalBinding, error) {
	if !a.MaintenanceEnabled() || r == nil || !a.AllowsRequest(r) || !a.Allows(old.Issuer, old.Subject) {
		return host.ExternalBinding{}, goauth.ErrSessionRevoked
	}
	client, ok := a.Client.(interface {
		Verify(request *http.Request) (browser.Proof, error)
	})
	if !ok {
		return host.ExternalBinding{}, goauth.ErrSessionRevoked
	}
	proof, err := client.Verify(r)
	if err != nil {
		if errors.Is(err, browser.ErrDenied) || errors.Is(err, browser.ErrRequiredAMR) {
			return host.ExternalBinding{}, goauth.ErrSessionRevoked
		}
		return host.ExternalBinding{}, err
	}
	return Build(proof, old.LocalSubject, old.LinkID)
}

// Build accepts only an opaque SDK proof. The result is protected server
// metadata; callers must never decode it from browser forms, headers or JSON.
// LinkID may be empty only while explicitly enrolling a verified identity.
func Build(proof browser.Proof, local goauth.SubjectID, linkID string) (host.ExternalBinding, error) {
	return buildSnapshot(proof.Binding(), proof.Identity(), local, linkID, time.Now())
}

func buildSnapshot(
	b browser.Binding, id browser.Identity, local goauth.SubjectID, linkID string, now time.Time,
) (host.ExternalBinding, error) {
	if local.IsZero() || b.Reference == "" || b.Generation <= 0 || b.LoginGeneration <= 0 ||
		b.Generation < b.LoginGeneration || b.Issuer == "" || b.Subject == "" || b.ProjectID == "" ||
		b.ClientID == "" || b.CentralSessionID == "" || b.AuthTime <= 0 ||
		b.Issuer != id.Issuer || b.Subject != id.Subject || b.ProjectID != id.ProjectID || b.ClientID != id.ClientID ||
		b.CentralSessionID != id.SessionID || b.AuthTime != id.AuthTime || !b.AbsoluteUntil.Equal(id.AbsoluteUntil) ||
		id.IssuedAt.IsZero() || id.IssuedAt.After(now) || time.Unix(b.AuthTime, 0).After(id.IssuedAt) ||
		!id.SessionExpiresAt.After(id.IssuedAt) || !b.AbsoluteUntil.After(id.IssuedAt) || b.Deadline.IsZero() {
		return host.ExternalBinding{}, goauth.ErrSessionRevoked
	}
	deadline := earliest(b.Deadline, id.IssuedAt.Add(freshnessCap), id.SessionExpiresAt, b.AbsoluteUntil)
	if !now.Before(deadline) || !deadline.After(id.IssuedAt) {
		return host.ExternalBinding{}, goauth.ErrSessionRevoked
	}
	return host.ExternalBinding{
		Issuer: b.Issuer, Subject: b.Subject, LinkID: linkID, AuthorityID: b.Reference,
		ProviderSessionID: b.CentralSessionID, ClientID: b.ClientID, ProjectID: b.ProjectID,
		ExternalAuthTime: time.Unix(b.AuthTime, 0), Generation: uint64(b.Generation), LoginGeneration: uint64(b.LoginGeneration),
		Deadline: deadline, LocalSubject: local, AbsoluteUntil: b.AbsoluteUntil,
		Proof: host.ExternalProof{IssuedAt: id.IssuedAt, ExpiresAt: id.SessionExpiresAt},
	}, nil
}

// Validate reads the admitted exact generation without refreshing or extending
// its deadline. Canonical linking and membership remain GoAdmin's responsibility.
func (a *Authority) Validate(ctx context.Context, binding host.ExternalBinding) (host.ExternalProof, error) {
	if a == nil || a.Client == nil || a.Allows == nil || ctx == nil {
		return host.ExternalProof{}, goauth.ErrSessionRevoked
	}
	b, err := sdkBinding(binding)
	if err != nil || !time.Now().Before(binding.Deadline) {
		return host.ExternalProof{}, goauth.ErrSessionRevoked
	}
	allowed := a.Allows(binding.Issuer, binding.Subject)
	if !allowed {
		return host.ExternalProof{}, goauth.ErrSessionRevoked
	}
	proof, err := a.Client.Check(ctx, b)
	if err != nil {
		if errors.Is(err, browser.ErrDenied) || errors.Is(err, browser.ErrRequiredAMR) {
			return host.ExternalProof{}, goauth.ErrSessionRevoked
		}
		return host.ExternalProof{}, err
	}
	expected, err := Build(proof, binding.LocalSubject, binding.LinkID)
	if err != nil || !sameSnapshot(expected, binding) {
		return host.ExternalProof{}, goauth.ErrSessionRevoked
	}

	return expected.Proof, nil
}

// Detach is session replacement, never intentional RP logout.
func (*Authority) Detach(context.Context, host.ExternalBinding) error { return nil }

// Logout revokes the pinned callback login directly, even after freshness or
// allowlist expiry. RevokeBinding owns durable fencing and bounded cleanup.
func (a *Authority) Logout(ctx context.Context, binding host.ExternalBinding) error {
	if a == nil || a.Client == nil || ctx == nil {
		return goauth.ErrSessionRevoked
	}
	b, err := sdkBinding(binding)
	if err != nil {
		return err
	}
	return a.Client.RevokeBinding(ctx, b)
}

func sdkBinding(b host.ExternalBinding) (browser.Binding, error) {
	if b.LocalSubject.IsZero() || b.AuthorityID == "" || b.Issuer == "" || b.Subject == "" ||
		b.ProjectID == "" || b.ClientID == "" || b.ProviderSessionID == "" ||
		b.Generation == 0 || b.LoginGeneration == 0 || b.Generation > math.MaxInt64 || b.LoginGeneration > b.Generation ||
		b.ExternalAuthTime.Unix() <= 0 || b.ExternalAuthTime.Nanosecond() != 0 || b.Proof.IssuedAt.IsZero() ||
		b.ExternalAuthTime.After(b.Proof.IssuedAt) || !b.Proof.ExpiresAt.After(b.Proof.IssuedAt) ||
		!b.Deadline.After(b.Proof.IssuedAt) || b.AbsoluteUntil.IsZero() ||
		b.Deadline.After(earliest(b.Proof.IssuedAt.Add(freshnessCap), b.Proof.ExpiresAt, b.AbsoluteUntil)) {
		return browser.Binding{}, goauth.ErrSessionRevoked
	}
	return browser.Binding{
		Reference: b.AuthorityID, Generation: int64(b.Generation), LoginGeneration: int64(b.LoginGeneration),
		Deadline: b.Deadline, Issuer: b.Issuer, Subject: b.Subject, ProjectID: b.ProjectID, ClientID: b.ClientID,
		CentralSessionID: b.ProviderSessionID, AuthTime: b.ExternalAuthTime.Unix(), AbsoluteUntil: b.AbsoluteUntil,
	}, nil
}

func sameSnapshot(a, b host.ExternalBinding) bool {
	return a.Issuer == b.Issuer && a.Subject == b.Subject && a.LinkID == b.LinkID && a.AuthorityID == b.AuthorityID &&
		a.ProviderSessionID == b.ProviderSessionID && a.ClientID == b.ClientID && a.ProjectID == b.ProjectID &&
		a.ExternalAuthTime.Equal(b.ExternalAuthTime) && a.Generation == b.Generation && a.LoginGeneration == b.LoginGeneration &&
		a.Deadline.Equal(b.Deadline) && a.LocalSubject == b.LocalSubject && a.AbsoluteUntil.Equal(b.AbsoluteUntil) &&
		a.Proof.IssuedAt.Equal(b.Proof.IssuedAt) && a.Proof.ExpiresAt.Equal(b.Proof.ExpiresAt)
}

func earliest(first time.Time, rest ...time.Time) time.Time {
	for _, value := range rest {
		if value.Before(first) {
			first = value
		}
	}
	return first
}
