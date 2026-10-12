// Package browserstate owns the server-side credentials behind an opaque admin
// cookie. Fiber's whole-session writes are not authoritative for these secrets.
package browserstate

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/assurrussa/goauth"
)

// LocalLoginPending is encrypted along with the browser journal. No canonical
// token or authenticated identity exists until the shared factor is completed.
type LocalLoginPending struct {
	ID         string              `json:"id"`
	Kind       goauth.MFALoginKind `json:"kind,omitempty"`
	Binding    [32]byte            `json:"binding"`
	ExpiresAt  time.Time           `json:"expiresAt"`
	Persistent bool                `json:"persistent"`
}
type Record struct {
	LocalLoginPending  *LocalLoginPending `json:"localLoginPending,omitempty"`
	LocalLoginTerminal bool               `json:"localLoginTerminal,omitempty"`
	//nolint:tagliatelle // Preserve the persisted journal field name across instances.
	LocalLoginSessionID string           `json:"localLoginSessionID,omitempty"`
	LocalLoginUntil     time.Time        `json:"localLoginUntil,omitempty"`
	Version             int64            `json:"version"`
	Owner               string           `json:"owner"`
	SubjectID           goauth.SubjectID `json:"subjectId"`
	Tokens              goauth.TokenPair `json:"tokens"`
}

// A refresh claim does not expire into another claim. A crashed or uncertain
// owner requires re-authentication; retrying the consumed secret is forbidden.
type Store interface {
	Load(ctx context.Context, id string) (Record, bool, error)
	Create(ctx context.Context, id string, record Record, ttl time.Duration) error
	Claim(ctx context.Context, id string, version int64, owner string) (bool, error)
	// ReleaseClaim is permitted ONLY before the canonical Refresh call. It does
	// not change tokens, version or expiry, and cannot release another owner.
	ReleaseClaim(ctx context.Context, id string, version int64, owner string) (bool, error)
	Complete(ctx context.Context, id string, version int64, owner string, record Record) (bool, error)
	Delete(ctx context.Context, id string) error
}

// Memory is for tests and single-process internal fixtures, not host assembly.
// Expiry is exercised against the real Redis/PostgreSQL implementations.
type Memory struct {
	mu      sync.Mutex
	records map[string]Record
}

func NewMemory() *Memory { return &Memory{records: make(map[string]Record)} }
func (s *Memory) Load(_ context.Context, id string) (Record, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.records[id]
	return r, ok, nil
}

func (s *Memory) Create(_ context.Context, id string, r Record, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.records[id]; exists {
		return errors.New("admin auth state already exists")
	}
	r.Version = 1
	r.Owner = ""
	s.records[id] = r
	return nil
}

func (s *Memory) Claim(_ context.Context, id string, v int64, owner string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.records[id]
	if !ok || r.Version != v || r.Owner != "" || owner == "" {
		return false, nil
	}
	r.Owner = owner
	s.records[id] = r
	return true, nil
}

func (s *Memory) Complete(_ context.Context, id string, v int64, owner string, r Record) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.records[id]
	if !ok || old.Version != v || old.Owner != owner || owner == "" {
		return false, nil
	}
	r.Version = v + 1
	r.Owner = ""
	s.records[id] = r
	return true, nil
}

func (s *Memory) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, id)
	return nil
}
