package mcp //nolint:testpackage // Test private login fences and pairing state.

import (
	"net/http"
	"net/netip"
	"testing"
	"time"

	"github.com/assurrussa/goadmin/host"
)

func TestActionPairingLifetimeAndSingleExchange(t *testing.T) {
	pairs := newActionPairings()
	now := time.Unix(100, 0)
	pairs.now = func() time.Time { return now }
	source := netip.MustParseAddr("2001:db8::1")
	code, secret, ok := pairs.start(source)
	if !ok || !codePattern.MatchString(code) {
		t.Fatal("pairing did not start")
	}
	if _, _, _, status := pairs.exchange(secret); status != http.StatusAccepted {
		t.Fatalf("unapproved pairing returned %d", status)
	}
	now = now.Add(4 * time.Minute)
	approvedAt := now
	browser := host.CSRFSessionBinding("synthetic-browser-session")
	grant := actionGrant{actor: host.Actor{AdminID: 1}, browser: browser, deadline: pairs.now().Add(time.Hour)}
	if !pairs.approve(code, grant) || pairs.approve(code, grant) {
		t.Fatal("the code must be approved only once")
	}
	now = now.Add(30 * time.Second)
	exchanged, token, expiresIn, status := pairs.exchange(secret)
	if status != http.StatusOK || token == "" || token == secret || expiresIn != 870 {
		t.Fatalf("exchange: status=%d expires_in=%d", status, expiresIn)
	}
	if _, _, _, status := pairs.exchange(secret); status != http.StatusGone {
		t.Fatalf("replayed device secret returned %d", status)
	}
	active, ok := pairs.bearer(token)
	if !ok || active != exchanged || !active.deadline.Equal(approvedAt.Add(15*time.Minute)) {
		t.Fatal("grant deadline must start at browser approval")
	}
	now = active.deadline
	if _, ok := pairs.bearer(token); ok {
		t.Fatal("bearer remained valid at the grant deadline")
	}
	if _, ok := pairs.bound(active.id); ok {
		t.Fatal("browser review binding remained live at the grant deadline")
	}
}

func TestActionLostExchangeCannotEraseAnotherPairing(t *testing.T) {
	pairs := newActionPairings()
	source := netip.MustParseAddr("127.0.0.1")
	code, secret, _ := pairs.start(source)
	_, unrelated, _ := pairs.start(source)
	browser := host.CSRFSessionBinding("synthetic-revoked-login")
	pairs.approve(code, actionGrant{browser: browser, deadline: pairs.now().Add(time.Hour)})
	exchanged, token, _, status := pairs.exchange(secret)
	if status != http.StatusOK || exchanged.browser != browser {
		t.Fatal("exchange did not capture the approved browser")
	}
	pairs.revoke(browser, "")
	lost, ok := pairs.bearer(token)
	if ok {
		t.Fatal("revoked token remained live")
	}
	pairs.revoke(lost.browser, lost.id)
	if _, _, _, status := pairs.exchange(unrelated); status != http.StatusAccepted {
		t.Fatalf("unrelated code was removed: HTTP %d", status)
	}
}

func TestActionRevocationRetainsOnlyOwnedCleanup(t *testing.T) {
	pairs := newActionPairings()
	source := netip.MustParseAddr("127.0.0.1")
	code, secret, started := pairs.start(source)
	if !started || !pairs.approve(code, actionGrant{
		browser: "first-browser", actor: host.Actor{AdminID: 1}, deadline: pairs.now().Add(time.Minute),
	}) {
		t.Fatal("could not pair first browser")
	}
	grant, _, _, status := pairs.exchange(secret)
	if status != http.StatusOK {
		t.Fatalf("exchange returned %d", status)
	}
	if retired := pairs.revoke("second-browser", grant.id); len(retired) != 0 {
		t.Fatal("another browser could revoke the grant")
	}
	retired := pairs.revoke("first-browser", grant.id)
	if len(retired) != 1 || retired[0].id != grant.id {
		t.Fatal("original browser could not retire the grant")
	}
	if _, ok := pairs.bound(grant.id); ok {
		t.Fatal("retired binding could still be used")
	}
	if retry := pairs.revoke("first-browser", grant.id); len(retry) != 1 {
		t.Fatal("original browser could not retry cleanup")
	}
	pairs.cleared(retired)
	if retry := pairs.revoke("first-browser", grant.id); len(retry) != 0 {
		t.Fatal("cleanup retained retired metadata")
	}
}

func TestActionLoginFenceNativeTransitionReleasesMutex(t *testing.T) {
	var fences actionLoginFences
	finish := fences.transition("browser")
	ready, release := fences.acquire("browser")
	release()
	if ready {
		t.Fatal("native transition admitted a concurrent action")
	}
	finish(func() {})
	ready, release = fences.acquire("browser")
	release()
	if !ready || len(fences.logins) != 0 {
		t.Fatal("completed transition left a login unavailable")
	}
}
