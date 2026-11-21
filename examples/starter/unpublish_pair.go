package main

import (
	"crypto/rand"
	"crypto/sha256"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/assurrussa/goadmin/host"
)

const (
	unpublishPairTTL     = 5 * time.Minute
	unpublishPairLimit   = 128
	unpublishSourceLimit = 4
)

type unpublishGrant struct {
	id       string
	actor    host.Actor
	browser  string
	session  host.ReadSessionBinding
	deadline time.Time
}

type unpublishPairing struct {
	code     string
	source   netip.Addr
	deadline time.Time
	grant    unpublishGrant
}

type unpublishPairings struct {
	mu      sync.Mutex
	now     func() time.Time
	pending map[[32]byte]unpublishPairing
	active  map[[32]byte]unpublishGrant
}

func newUnpublishPairings() *unpublishPairings {
	return &unpublishPairings{
		now: time.Now, pending: make(map[[32]byte]unpublishPairing), active: make(map[[32]byte]unpublishGrant),
	}
}

func unpublishSecretKey(secret string) [32]byte { return sha256.Sum256([]byte(secret)) }

func (p *unpublishPairings) prune(now time.Time) {
	for key, pair := range p.pending {
		if !now.Before(pair.deadline) {
			delete(p.pending, key)
		}
	}
	for key, grant := range p.active {
		if !now.Before(grant.deadline) {
			delete(p.active, key)
		}
	}
}

func (p *unpublishPairings) start(source netip.Addr) (string, string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	p.prune(now)
	if len(p.pending)+len(p.active) >= unpublishPairLimit {
		return "", "", false
	}
	if source.Is6() {
		source = netip.PrefixFrom(source, 64).Masked().Addr()
	}
	codes := make(map[string]bool, len(p.pending))
	sameSource := 0
	for _, pair := range p.pending {
		codes[pair.code] = true
		if pair.source == source {
			sameSource++
		}
	}
	if sameSource >= unpublishSourceLimit {
		return "", "", false
	}
	for {
		raw := strings.ToUpper(rand.Text()[:10])
		code := raw[:5] + "-" + raw[5:]
		if !codes[code] {
			secret := rand.Text()
			p.pending[unpublishSecretKey(secret)] = unpublishPairing{code: code, source: source, deadline: now.Add(unpublishPairTTL)}
			return code, secret, true
		}
	}
}

func (p *unpublishPairings) approve(code string, grant unpublishGrant) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	p.prune(now)
	for key, pair := range p.pending {
		if pair.code == code && pair.grant.id == "" {
			grant.id = rand.Text()
			grant.deadline = now.Add(unpublishMaxLifetime)
			pair.grant = grant
			p.pending[key] = pair
			return true
		}
	}
	return false
}

func (p *unpublishPairings) exchange(secret string) (unpublishGrant, string, int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	p.prune(now)
	key := unpublishSecretKey(secret)
	pair, ok := p.pending[key]
	if !ok {
		return unpublishGrant{}, "", 0, http.StatusGone
	}
	if pair.grant.id == "" {
		return unpublishGrant{}, "", 0, http.StatusAccepted
	}
	delete(p.pending, key)
	token := rand.Text()
	p.active[unpublishSecretKey(token)] = pair.grant
	return pair.grant, token, int(pair.grant.deadline.Sub(now).Seconds()), http.StatusOK
}

func (p *unpublishPairings) bearer(token string) (unpublishGrant, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prune(p.now())
	grant, ok := p.active[unpublishSecretKey(token)]
	return grant, ok
}

func (p *unpublishPairings) bound(id string) (unpublishGrant, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prune(p.now())
	for _, grant := range p.active {
		if grant.id == id {
			return grant, true
		}
	}
	return unpublishGrant{}, false
}

func (p *unpublishPairings) connections(browser string) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prune(p.now())
	var ids []string
	for _, pair := range p.pending {
		if pair.grant.browser == browser {
			ids = append(ids, pair.grant.id)
		}
	}
	for _, grant := range p.active {
		if grant.browser == browser {
			ids = append(ids, grant.id)
		}
	}
	slices.Sort(ids)
	return ids
}

// An empty id closes all connections for a browser. A nonempty id is private
// connection metadata, never accepted as a device secret or an access token.
func (p *unpublishPairings) revoke(browser, id string) {
	if browser == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for key, pair := range p.pending {
		if pair.grant.browser == browser && (id == "" || pair.grant.id == id) {
			delete(p.pending, key)
		}
	}
	for key, grant := range p.active {
		if grant.browser == browser && (id == "" || grant.id == id) {
			delete(p.active, key)
		}
	}
}

// The fence for one authenticated login only covers already-locked database
// mutations and native session changes. A disconnect can win while a review
// is still waiting on a product/request lock. Idle fences are discarded.
type unpublishLoginFences struct {
	mu     sync.Mutex
	logins map[string]*unpublishLoginFence
}

type unpublishLoginFence struct {
	mu   sync.Mutex
	refs int
}

func (f *unpublishLoginFences) acquire(browser string) func() {
	f.mu.Lock()
	if f.logins == nil {
		f.logins = make(map[string]*unpublishLoginFence)
	}
	fence := f.logins[browser]
	if fence == nil {
		fence = &unpublishLoginFence{}
		f.logins[browser] = fence
	}
	fence.refs++
	f.mu.Unlock()
	fence.mu.Lock()
	return func() {
		fence.mu.Unlock()
		f.mu.Lock()
		fence.refs--
		if fence.refs == 0 {
			delete(f.logins, browser)
		}
		f.mu.Unlock()
	}
}
