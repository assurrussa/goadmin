package mcp //nolint:testpackage // Exercise the unexported MCP transport and grant state.

import (
	"net/http"
	"net/netip"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/assurrussa/goadmin/host"
)

const testOwnerSubject = "owner"

func TestJournalGrantOwnerAndExpiry(t *testing.T) { //nolint:gocyclo // One grant lifecycle is checked end to end.
	store := newGrantStore()
	now := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	owner := host.Actor{AdminID: 1, SubjectID: testOwnerSubject}
	other := host.Actor{AdminID: 2, SubjectID: "other"}
	var pin host.ReadSessionBinding // Only the host's session service creates usable pins.

	code, secret, ok := store.start(netip.IPv6Loopback())
	if !ok || code == "" || secret == "" || code == secret {
		t.Fatal("expected distinct verification code and device secret")
	}
	if _, _, status := store.exchange(code); status != http.StatusGone {
		t.Fatal("verification code must not be accepted for token exchange")
	}
	if _, _, status := store.exchange(secret); status != http.StatusAccepted {
		t.Fatal("new connection should await approval")
	}
	if store.decide(code, true, host.Actor{}, pin) || len(store.active(owner)) != 0 {
		t.Fatal("unapproved code exposed or approved without an operator")
	}
	now = now.Add(time.Minute)
	if !store.decide(code, true, owner, pin) || store.decide(code, true, other, pin) {
		t.Fatal("approval must bind exactly one owner")
	}
	if !slices.Equal(store.active(owner), []string{code}) || len(store.active(other)) != 0 || store.revoke(code, other) {
		t.Fatal("pending approval must be visible and revocable only by its owner")
	}
	now = now.Add(time.Minute)
	token, lifetime, status := store.exchange(secret)
	if token == "" || token == code || token == secret || status != http.StatusOK || lifetime != 14*60 {
		t.Fatal("exchange must preserve the deadline set at approval")
	}
	if reused, _, status := store.exchange(secret); reused != "" || status != http.StatusGone {
		t.Fatal("consumed exchange returned a second token")
	}
	if got, ok := store.lookup(token); !ok || got.owner != owner || got.code != code {
		t.Fatal("issued token lost its binding")
	}
	if !slices.Equal(store.active(owner), []string{code}) || len(store.active(other)) != 0 || store.revoke(code, other) {
		t.Fatal("active token ownership differs from its approval")
	}
	now = now.Add(14 * time.Minute)
	if _, ok := store.lookup(token); ok || len(store.active(owner)) != 0 || store.revoke(code, owner) {
		t.Fatal("a grant remained active at its expiry")
	}
	code, secret, ok = store.start(netip.IPv6Loopback())
	if !ok || !store.decide(code, false, owner, pin) || store.decide(code, true, owner, pin) {
		t.Fatal("denied pairing could be approved again")
	}
	if _, _, status := store.exchange(secret); status != http.StatusForbidden {
		t.Fatal("client did not observe denial")
	}
	if _, _, status := store.exchange(secret); status != http.StatusGone {
		t.Fatal("denied exchange must also be consumed")
	}
	code, secret, ok = store.start(netip.IPv6Loopback())
	now = now.Add(pairLifetime)
	if !ok || store.decide(code, true, owner, pin) {
		t.Fatal("expired pending code was approved")
	}
	if _, _, status := store.exchange(secret); status != http.StatusGone {
		t.Fatal("expired pending proof was accepted")
	}
}

func TestJournalGrantConcurrentDecisionsAndExchange(t *testing.T) {
	store := newGrantStore()
	code, secret, ok := store.start(netip.IPv6Loopback())
	if !ok {
		t.Fatal("pairing unavailable")
	}
	actors := []host.Actor{{AdminID: 1, SubjectID: "one"}, {AdminID: 2, SubjectID: "two"}}
	approved := make([]bool, len(actors))
	barrier := make(chan struct{})
	var workers sync.WaitGroup
	for i, actor := range actors {
		workers.Go(func() {
			<-barrier
			approved[i] = store.decide(code, true, actor, host.ReadSessionBinding{})
		})
	}
	close(barrier)
	workers.Wait()
	if approved[0] == approved[1] {
		t.Fatal("concurrent approvals must choose exactly one owner")
	}
	winner := actors[slices.Index(approved, true)]
	start := make(chan struct{})
	type exchange struct {
		token  string
		status int
	}
	results := make([]exchange, 10)
	for i := range results {
		workers.Go(func() {
			<-start
			results[i].token, _, results[i].status = store.exchange(secret)
		})
	}
	close(start)
	workers.Wait()
	success := 0
	for _, r := range results {
		if r.status == http.StatusGone && r.token == "" {
			continue
		}
		grant, found := store.lookup(r.token)
		if r.status != http.StatusOK || !found || grant.owner != winner {
			t.Fatal("unexpected result of concurrent exchange")
		}
		success++
	}
	if success != 1 || !store.revoke(code, winner) || len(store.active(winner)) != 0 {
		t.Fatal("expected one issued and revoked token")
	}
}

func TestJournalPairingQuotaFollowsPendingDecisions(t *testing.T) {
	owner := host.Actor{AdminID: 1, SubjectID: testOwnerSubject}
	for _, approve := range []bool{true, false} {
		store := newGrantStore()
		first := netip.MustParseAddr("2001:db8:1::10")
		sameSubnet := netip.MustParseAddr("2001:db8:1::20")
		var code string
		for range maxPendingPerSource {
			var ok bool
			code, _, ok = store.start(first)
			if !ok {
				t.Fatal("source quota exhausted early")
			}
		}
		if _, _, ok := store.start(sameSubnet); ok {
			t.Fatal("IPv6 subnet exceeded its pending quota")
		}
		if _, _, ok := store.start(netip.MustParseAddr("2001:db8:2::10")); !ok {
			t.Fatal("independent IPv6 subnet refused")
		}
		if !store.decide(code, approve, owner, host.ReadSessionBinding{}) {
			t.Fatal("operator decision refused")
		}
		if _, _, ok := store.start(sameSubnet); !ok {
			t.Fatalf("decision (approve=%t) did not release the source slot", approve)
		}
	}
}

func TestJournalPairingPreservesGlobalCapacity(t *testing.T) {
	store := newGrantStore()
	expires := store.now().Add(grantLifetime)
	for i := range maxAgentRequests - 1 {
		store.tokens[[32]byte{byte(i)}] = readGrant{expires: expires}
	}
	if _, _, ok := store.start(netip.MustParseAddr("192.0.2.10")); !ok {
		t.Fatal("last available slot refused")
	}
	if _, _, ok := store.start(netip.MustParseAddr("192.0.2.20")); ok {
		t.Fatal("independent source exceeded the shared memory limit")
	}
}

func TestJournalPairingReservesLastSourceSlotOnce(t *testing.T) {
	store := newGrantStore()
	source := netip.MustParseAddr("192.0.2.10")
	for range maxPendingPerSource - 1 {
		if _, _, ok := store.start(source); !ok {
			t.Fatal("could not reserve pending pairing")
		}
	}
	ready := make(chan struct{})
	result := make(chan bool, 2)
	for range 2 {
		go func() {
			<-ready
			_, _, ok := store.start(source)
			result <- ok
		}()
	}
	close(ready)
	first, second := <-result, <-result
	if first == second {
		t.Fatalf("expected exactly one reservation for the last slot, got %t and %t", first, second)
	}
}

func TestJournalGrantRevokeRacingExchange(t *testing.T) {
	owner := host.Actor{AdminID: 1, SubjectID: testOwnerSubject}
	for range 32 {
		store := newGrantStore()
		code, secret, ok := store.start(netip.IPv6Loopback())
		if !ok || !store.decide(code, true, owner, host.ReadSessionBinding{}) {
			t.Fatal("failed to approve pairing")
		}
		start := make(chan struct{})
		var token string
		var status int
		var revoked bool
		var workers sync.WaitGroup
		workers.Go(func() {
			<-start
			token, _, status = store.exchange(secret)
		})
		workers.Go(func() {
			<-start
			revoked = store.revoke(code, owner)
		})
		close(start)
		workers.Wait()
		if !revoked || (status != http.StatusOK && status != http.StatusForbidden) {
			t.Fatal("unexpected exchange/revoke result", status, revoked)
		}
		if _, ok := store.lookup(token); ok || len(store.active(owner)) != 0 {
			t.Fatal("a grant survived its revocation")
		}
	}
}
