package admission //nolint:testpackage // White-box tests cover unmounted guards and transaction ordering.

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"testing"
	"time"
)

type fixtureVerifier struct {
	identity Identity
	denied   bool
}

func (v *fixtureVerifier) Verify(context.Context, string) (Identity, error) {
	if v.denied {
		return Identity{}, ErrDenied
	}
	return v.identity, nil
}

func (v *fixtureVerifier) Revalidate(context.Context, string, Identity) error {
	if v.denied {
		return ErrDenied
	}
	return nil
}

type fixtureTx struct {
	records    map[string]Record
	invites    map[string]Invitation
	identities map[string]string
	members    map[string]bool
	blocked    map[string]bool
	roles      []Role
	grant      bool
	creates    int
	audits     int
}
type fixtureStore struct {
	mu          sync.Mutex
	state       fixtureTx
	commitError error
}

func (s *fixtureStore) Owned(ctx context.Context, fn func(context.Context, Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.state
	next.records = maps.Clone(next.records)
	next.invites = maps.Clone(next.invites)
	next.identities = maps.Clone(next.identities)
	next.members = maps.Clone(next.members)
	if err := fn(ctx, &next); err != nil {
		return err
	}
	s.state = next
	return s.commitError
}
func (*fixtureTx) LockIdentity(context.Context, Scope, string) error { return nil }
func (t *fixtureTx) Find(_ context.Context, _ Scope, subject string) (Record, error) {
	r, ok := t.records[subject]
	if !ok {
		return Record{}, ErrNotFound
	}
	return r, nil
}

func (t *fixtureTx) Save(_ context.Context, r Record, expected int64) (Record, error) {
	old, exists := t.records[r.ExternalSubject]
	if exists && old.Version != expected || !exists && expected != 0 {
		return Record{}, ErrConflict
	}
	r.Version = expected + 1
	t.records[r.ExternalSubject] = r
	return r, nil
}

func (t *fixtureTx) Invitation(_ context.Context, id string) (Invitation, error) {
	i, ok := t.invites[id]
	if !ok {
		return Invitation{}, ErrNotFound
	}
	return i, nil
}

func (t *fixtureTx) CreateInvitation(_ context.Context, i Invitation) error {
	if _, ok := t.invites[i.ID]; ok {
		return ErrConflict
	}
	t.invites[i.ID] = i
	return nil
}

func (t *fixtureTx) ConsumeInvitation(_ context.Context, id, subject string) error {
	i, ok := t.invites[id]
	if !ok || i.Revoked || i.AcceptedSubject != "" {
		return ErrConflict
	}
	i.AcceptedSubject = subject
	t.invites[id] = i
	return nil
}

func (t *fixtureTx) RevokeInvitation(_ context.Context, id string) error {
	i, ok := t.invites[id]
	if !ok || i.AcceptedSubject != "" {
		return ErrConflict
	}
	i.Revoked = true
	t.invites[id] = i
	return nil
}

func (t *fixtureTx) Authorize(_ context.Context, actor, _ string, ids []int64) ([]Role, error) {
	if actor != "operator" || !t.grant {
		return nil, ErrDenied
	}
	out := []Role{}
	for _, id := range ids {
		found := false
		for _, role := range t.roles {
			if role.ID == id {
				out = append(out, role)
				found = true
			}
		}
		if !found {
			return nil, ErrDenied
		}
	}
	return out, nil
}

func (t *fixtureTx) Provision(_ context.Context, i Identity) (string, error) {
	if t.blocked[i.Subject] {
		return "", ErrDenied
	}
	if id, ok := t.identities[i.Subject]; ok {
		return id, nil
	}
	id := "local-" + i.Subject
	t.identities[i.Subject] = id
	t.creates++
	return id, nil
}

func (t *fixtureTx) Activate(_ context.Context, id string, _ []Role) error {
	if t.members[id] {
		return ErrConflict
	}
	t.members[id] = true
	return nil
}

func (t *fixtureTx) CheckActive(_ context.Context, id string) error {
	if !t.members[id] {
		return ErrDenied
	}
	return nil
}

func (t *fixtureTx) CheckIdentity(_ context.Context, i Identity, id string) error {
	if t.blocked[i.Subject] || t.identities[i.Subject] != id {
		return ErrDenied
	}
	return nil
}
func (t *fixtureTx) Audit(context.Context, string, string, string) error { t.audits++; return nil }

func fixture(t *testing.T, mode Mode) (*Service, *fixtureStore, *fixtureVerifier, *time.Time) {
	t.Helper()
	now := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	scope := Scope{Issuer: "https://issuer.example.test", Project: "project", Client: "client"}
	v := &fixtureVerifier{identity: Identity{
		Scope: scope, Subject: "alice", Email: "alice@example.test",
		EmailVerified: true, ExpiresAt: now.Add(time.Hour),
	}}
	db := &fixtureStore{state: fixtureTx{
		records: map[string]Record{}, invites: map[string]Invitation{},
		identities: map[string]string{}, members: map[string]bool{}, blocked: map[string]bool{},
		roles: []Role{{ID: 1, PublicID: "role-one", Name: "Reader", Fingerprint: "read"}}, grant: true,
	}}
	seq := 0
	service, err := New(Config{
		Mode: mode, Scope: scope, Now: func() time.Time { return now },
		NewID: func() string { seq++; return fmt.Sprintf("id-%d", seq) },
	}, db, v)
	if err != nil {
		t.Fatal(err)
	}
	return service, db, v, &now
}

func TestNoGrantNeverProvisionsOrConsumesInvitation(t *testing.T) {
	s, db, v, now := fixture(t, InvitationOnly)
	i, err := s.Invite(context.Background(), "operator", "alice", []int64{1}, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	v.denied = true
	if _, err = s.Admit(context.Background(), "rp", i.ID); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	if db.state.creates != 0 || db.state.invites[i.ID].AcceptedSubject != "" {
		t.Fatal("denial mutated enrollment")
	}
	v.denied = false
	r, err := s.Admit(context.Background(), "fresh-rp", i.ID)
	if err != nil || r.Status != Active {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestPendingDoesNotCreateMembershipAndApprovalRequiresFreshAdmission(t *testing.T) {
	s, db, v, now := fixture(t, ProjectPending)
	r, err := s.Admit(context.Background(), "rp", "")
	if err != nil || r.Status != Pending || len(db.state.members) != 0 {
		t.Fatalf("%+v %v", r, err)
	}
	if err = s.Approve(context.Background(), "operator", "alice", r.Version, []int64{1}, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if len(db.state.members) != 0 {
		t.Fatal("approval granted membership before fresh proof")
	}
	v.denied = true
	if _, err = s.Admit(context.Background(), "stale-rp", ""); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	v.denied = false
	r, err = s.Admit(context.Background(), "fresh-rp", "")
	if err != nil || r.Status != Active || !db.state.members[r.LocalSubject] {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestInvitationRechecksIdentityExpiryRevocationAndGrant(t *testing.T) {
	for _, name := range []string{
		"wrong-subject", "expired", "revoked", "grant-revoked", "role-expanded", "wrong-project", "blocked",
	} {
		t.Run(name, func(t *testing.T) {
			s, db, v, now := fixture(t, InvitationOnly)
			i, err := s.Invite(context.Background(), "operator", "alice", []int64{1}, now.Add(time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			switch name {
			case "wrong-subject":
				v.identity.Subject = "bob"
			case "expired":
				*now = now.Add(2 * time.Minute)
			case "revoked":
				i.Revoked = true
				db.state.invites[i.ID] = i
			case "grant-revoked":
				db.state.grant = false
			case "role-expanded":
				db.state.roles[0].Fingerprint = "read+write"
			case "wrong-project":
				v.identity.Scope.Project = "other"
			case "blocked":
				db.state.blocked["alice"] = true
			}
			if _, err = s.Admit(context.Background(), "rp", i.ID); err == nil {
				t.Fatal("unexpected admission")
			}
			if db.state.creates != 0 || len(db.state.members) != 0 || db.state.invites[i.ID].AcceptedSubject != "" {
				t.Fatal("denial mutated authority")
			}
		})
	}
}

func TestRepeatedAndConcurrentCallbacksHaveOneAccountAndGrant(t *testing.T) {
	s, db, _, now := fixture(t, InvitationOnly)
	i, err := s.Invite(context.Background(), "operator", "alice", []int64{1}, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.Admit(context.Background(), "rp", i.ID); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if db.state.creates != 1 || len(db.state.members) != 1 {
		t.Fatal("duplicate account or membership")
	}
	delete(db.state.members, "local-alice")
	if _, err = s.Admit(context.Background(), "rp", i.ID); !errors.Is(err, ErrDenied) {
		t.Fatal("revoked membership restored", err)
	}
}

func TestRejectedAndBlockedPendingCannotReactivate(t *testing.T) {
	s, db, _, _ := fixture(t, ProjectPending)
	r, err := s.Admit(context.Background(), "rp", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Reject(context.Background(), "operator", "alice", r.Version); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Admit(context.Background(), "rp", ""); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	db.state.blocked["alice"] = true
	if _, err = s.View(context.Background(), "rp"); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
}

func TestIssuerRevocationAfterApprovalFailsClosed(t *testing.T) {
	s, db, _, now := fixture(t, ProjectPending)
	r, err := s.Admit(context.Background(), "rp", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Approve(context.Background(), "operator", "alice", r.Version, []int64{1}, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	db.state.grant = false
	if _, err = s.Admit(context.Background(), "rp", ""); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	if len(db.state.members) != 0 {
		t.Fatal("revoked issuer granted membership")
	}
}

func TestUnknownCommitNeverReturnsSuccess(t *testing.T) {
	s, db, _, _ := fixture(t, ProjectPending)
	db.commitError = errors.New("unknown commit")
	r, err := s.Admit(context.Background(), "rp", "")
	if err == nil || r.ID != "" || r.LocalSubject != "" {
		t.Fatal("provisional result escaped")
	}
}

func TestActorWithoutExplicitPermissionCannotInvite(t *testing.T) {
	s, db, _, now := fixture(t, InvitationOnly)
	_, err := s.Invite(context.Background(), "super-role-without-grant", "alice", []int64{1}, now.Add(time.Hour))
	if !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	if len(db.state.invites) != 0 {
		t.Fatal("unauthorized invitation")
	}
}

func TestRoleIDsAreBoundedSortedAndCopied(t *testing.T) {
	input := []int64{2, 1, 2}
	got, err := normalizedIDs(input)
	if err != nil || !slices.Equal(got, []int64{1, 2}) || !slices.Equal(input, []int64{2, 1, 2}) {
		t.Fatal(got, err)
	}
	for _, ids := range [][]int64{nil, {0}, {-1}, make([]int64, 33)} {
		if _, err := normalizedIDs(ids); err == nil {
			t.Fatal("invalid role set accepted")
		}
	}
}

type orderedRevokeStore struct {
	Store
	calls *[]string
}
type orderedRevokeTx struct {
	Tx
	calls *[]string
}

func (s orderedRevokeStore) Owned(ctx context.Context, fn func(context.Context, Tx) error) error {
	return s.Store.Owned(ctx, func(ctx context.Context, tx Tx) error {
		return fn(ctx, orderedRevokeTx{Tx: tx, calls: s.calls})
	})
}

func (t orderedRevokeTx) Invitation(ctx context.Context, id string) (Invitation, error) {
	*t.calls = append(*t.calls, "invitation")
	return t.Tx.Invitation(ctx, id)
}

func (t orderedRevokeTx) Authorize(ctx context.Context, actor, action string, ids []int64) ([]Role, error) {
	*t.calls = append(*t.calls, "authority")
	return t.Tx.Authorize(ctx, actor, action, ids)
}

func TestRevokeLocksInvitationBeforeAuthority(t *testing.T) {
	s, db, _, now := fixture(t, InvitationOnly)
	i, err := s.Invite(context.Background(), "operator", "alice", []int64{1}, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	calls := []string{}
	s.store = orderedRevokeStore{Store: db, calls: &calls}
	if err := s.Revoke(context.Background(), "operator", i.ID); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(calls, []string{"invitation", "authority"}) {
		t.Fatalf("lock order: %v", calls)
	}
	if !db.state.invites[i.ID].Revoked {
		t.Fatal("invitation not revoked")
	}
}
