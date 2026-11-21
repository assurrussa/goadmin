package mcp

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
	actionPairTTL     = 5 * time.Minute
	actionPairLimit   = 128
	actionSourceLimit = 4
)

type actionGrant struct {
	id       string
	actor    host.Actor
	browser  string
	session  host.ReadSessionBinding
	deadline time.Time
}

type actionPairing struct {
	code     string
	source   netip.Addr
	deadline time.Time
	grant    actionGrant
}

type actionPairings struct {
	mu      sync.Mutex
	now     func() time.Time
	pending map[[32]byte]actionPairing
	active  map[[32]byte]actionGrant
	retired map[string]actionGrant
}

func newActionPairings() *actionPairings {
	return &actionPairings{
		now: time.Now, pending: make(map[[32]byte]actionPairing), active: make(map[[32]byte]actionGrant),
		retired: make(map[string]actionGrant),
	}
}

func actionSecretKey(secret string) [32]byte { return sha256.Sum256([]byte(secret)) }

func (p *actionPairings) prune(now time.Time) {
	for key, pair := range p.pending {
		if !now.Before(pair.deadline) || (pair.grant.id != "" && !now.Before(pair.grant.deadline)) {
			delete(p.pending, key)
		}
	}
	for key, grant := range p.active {
		if !now.Before(grant.deadline) {
			delete(p.active, key)
		}
	}
	for id, grant := range p.retired {
		if !now.Before(grant.deadline) {
			delete(p.retired, id)
		}
	}
}

func (p *actionPairings) start(source netip.Addr) (pairCode, deviceSecret string, started bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	p.prune(now)
	if len(p.pending)+len(p.active) >= actionPairLimit {
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
	if sameSource >= actionSourceLimit {
		return "", "", false
	}
	for {
		raw := strings.ToUpper(rand.Text()[:10])
		code := raw[:5] + "-" + raw[5:]
		if !codes[code] {
			secret := rand.Text()
			p.pending[actionSecretKey(secret)] = actionPairing{code: code, source: source, deadline: now.Add(actionPairTTL)}
			return code, secret, true
		}
	}
}

func (p *actionPairings) approve(code string, grant actionGrant) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	p.prune(now)
	for key, pair := range p.pending {
		if pair.code == code && pair.grant.id == "" {
			if grant.deadline.IsZero() || !now.Before(grant.deadline) {
				return false
			}
			grant.id = rand.Text()
			if maxDeadline := now.Add(grantLifetime); grant.deadline.After(maxDeadline) {
				grant.deadline = maxDeadline
			}
			pair.grant = grant
			p.pending[key] = pair
			return true
		}
	}
	return false
}

func (p *actionPairings) exchange(secret string) (approved actionGrant, bearer string, remaining, status int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	p.prune(now)
	key := actionSecretKey(secret)
	pair, ok := p.pending[key]
	if !ok {
		return actionGrant{}, "", 0, http.StatusGone
	}
	if pair.grant.id == "" {
		return actionGrant{}, "", 0, http.StatusAccepted
	}
	delete(p.pending, key)
	token := rand.Text()
	p.active[actionSecretKey(token)] = pair.grant
	return pair.grant, token, int(pair.grant.deadline.Sub(now).Seconds()), http.StatusOK
}

func (p *actionPairings) bearer(token string) (actionGrant, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prune(p.now())
	grant, ok := p.active[actionSecretKey(token)]
	return grant, ok
}

func (p *actionPairings) bound(id string) (actionGrant, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prune(p.now())
	for _, grant := range p.active {
		if grant.id == id {
			return grant, true
		}
	}
	return actionGrant{}, false
}

func (p *actionPairings) connections(browser string) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prune(p.now())
	ids := make([]string, 0)
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
func (p *actionPairings) revoke(browser, id string) []actionGrant {
	if browser == "" {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prune(p.now())
	for key, pair := range p.pending {
		if pair.grant.browser == browser && (id == "" || pair.grant.id == id) {
			delete(p.pending, key)
		}
	}
	for key, grant := range p.active {
		if grant.browser == browser && (id == "" || grant.id == id) {
			delete(p.active, key)
			grant.session = host.ReadSessionBinding{}
			p.retired[grant.id] = grant
		}
	}
	var retired []actionGrant
	for _, grant := range p.retired {
		if grant.browser == browser && (id == "" || grant.id == id) {
			retired = append(retired, grant)
		}
	}
	return retired
}

func (p *actionPairings) cleared(grants []actionGrant) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, grant := range grants {
		delete(p.retired, grant.id)
	}
}

// The fence for one authenticated login only covers already-locked database
// mutations and native session changes. A disconnect can win while a review
// is still waiting on a product/request lock. Idle fences are discarded.
type actionLoginFences struct {
	mu     sync.Mutex
	logins map[string]*actionLoginFence
}

type actionLoginFence struct {
	mu          sync.Mutex
	refs        int
	transitions int
}

func (f *actionLoginFences) reference(browser string) (*actionLoginFence, func()) {
	f.mu.Lock()
	if f.logins == nil {
		f.logins = make(map[string]*actionLoginFence)
	}
	fence := f.logins[browser]
	if fence == nil {
		fence = &actionLoginFence{}
		f.logins[browser] = fence
	}
	fence.refs++
	f.mu.Unlock()
	return fence, func() {
		f.mu.Lock()
		fence.refs--
		if fence.refs == 0 {
			delete(f.logins, browser)
		}
		f.mu.Unlock()
	}
}

func (f *actionLoginFences) acquire(browser string) (ready bool, release func()) {
	fence, unref := f.reference(browser)
	fence.mu.Lock()
	return fence.transitions == 0, func() {
		fence.mu.Unlock()
		unref()
	}
}

// Native transitions mark the login unavailable without keeping the mutex
// while native auth waits for PostgreSQL.
func (f *actionLoginFences) transition(browser string) func(func()) {
	fence, unref := f.reference(browser)
	fence.mu.Lock()
	fence.transitions++
	fence.mu.Unlock()
	return func(beforeRelease func()) {
		fence.mu.Lock()
		defer unref()
		defer fence.mu.Unlock()
		beforeRelease()
		fence.transitions--
	}
}
