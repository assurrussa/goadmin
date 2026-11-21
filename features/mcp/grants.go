package mcp

import (
	"crypto/rand"
	"crypto/sha256"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/assurrussa/goadmin/host"
)

const (
	pairLifetime        = 5 * time.Minute
	grantLifetime       = 15 * time.Minute
	maxAgentRequests    = 128
	maxPendingPerSource = 4
)

type readGrant struct {
	code    string
	owner   host.Actor
	binding host.ReadSessionBinding
	expires time.Time
}

type pairing struct {
	pendingSource netip.Addr
	code          string
	until         time.Time
	approved      *readGrant
	denied        bool
}

// All transitions, including a one-time exchange racing revocation, hold mu.
// Only the local process receives the device secret and bearer token.
type grantStore struct {
	mu      sync.Mutex
	now     func() time.Time
	devices map[[32]byte]*pairing
	tokens  map[[32]byte]readGrant
}

func newGrantStore() *grantStore {
	return &grantStore{now: time.Now, devices: make(map[[32]byte]*pairing), tokens: make(map[[32]byte]readGrant)}
}

func secretKey(s string) [32]byte { return sha256.Sum256([]byte(s)) }

func (g *grantStore) prune(now time.Time) {
	for key, p := range g.devices {
		if !p.until.After(now) {
			delete(g.devices, key)
		}
	}
	for key, grant := range g.tokens {
		if !grant.expires.After(now) {
			delete(g.tokens, key)
		}
	}
}

func (g *grantStore) start(source netip.Addr) (code, secret string, ok bool) {
	if !source.IsValid() {
		return "", "", false
	}
	source = source.Unmap()
	if source.Is6() {
		// Treat privacy addresses on one IPv6 subnet as a single source.
		source = netip.PrefixFrom(source, 64).Masked().Addr()
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	g.prune(now)
	if len(g.devices)+len(g.tokens) >= maxAgentRequests {
		return "", "", false
	}
	// The global cap bounds this scan. Keep no source state beyond pending codes.
	pending := 0
	for _, device := range g.devices {
		if device.pendingSource == source {
			pending++
		}
	}
	if pending >= maxPendingPerSource {
		return "", "", false
	}
	for {
		raw := strings.ToUpper(rand.Text()[:10])
		code = raw[:5] + "-" + raw[5:]
		if !g.codeInUse(code) {
			break
		}
	}
	secret = rand.Text()
	g.devices[secretKey(secret)] = &pairing{pendingSource: source, code: code, until: now.Add(pairLifetime)}
	return code, secret, true
}

func (g *grantStore) codeInUse(code string) bool {
	if g.byCode(code) != nil {
		return true
	}
	for _, grant := range g.tokens {
		if grant.code == code {
			return true
		}
	}
	return false
}

func (g *grantStore) byCode(code string) *pairing {
	for _, p := range g.devices {
		if p.code == code {
			return p
		}
	}
	return nil
}

func (g *grantStore) decide(code string, approve bool, actor host.Actor, binding host.ReadSessionBinding) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	g.prune(now)
	p := g.byCode(code)
	if p == nil || p.approved != nil || p.denied || actor.IsZero() {
		return false
	}
	p.pendingSource = netip.Addr{}
	if approve {
		p.approved = &readGrant{code: p.code, owner: actor, binding: binding, expires: now.Add(grantLifetime)}
	} else {
		p.denied = true
	}
	return true
}

func (g *grantStore) exchange(secret string) (token string, seconds int, status int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	g.prune(now)
	key := secretKey(secret)
	p := g.devices[key]
	switch {
	case p == nil:
		return "", 0, 410
	case p.denied:
		delete(g.devices, key)
		return "", 0, 403
	case p.approved == nil:
		return "", 0, 202
	}
	token = rand.Text()
	g.tokens[secretKey(token)] = *p.approved
	delete(g.devices, key)
	return token, int(p.approved.expires.Sub(now).Seconds()), 200
}

func (g *grantStore) lookup(token string) (readGrant, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.prune(g.now())
	grant, ok := g.tokens[secretKey(token)]
	return grant, ok
}

func (g *grantStore) active(owner host.Actor) []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.prune(g.now())
	codes := []string{}
	for _, p := range g.devices {
		if p.approved != nil && p.approved.owner == owner {
			codes = append(codes, p.code)
		}
	}
	for _, grant := range g.tokens {
		if grant.owner == owner {
			codes = append(codes, grant.code)
		}
	}
	slices.Sort(codes)
	return codes
}

func (g *grantStore) revoke(code string, owner host.Actor) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.prune(g.now())
	if p := g.byCode(code); p != nil && p.approved != nil && p.approved.owner == owner {
		p.approved = nil
		p.denied = true
		return true
	}
	for key, grant := range g.tokens {
		if grant.code == code && grant.owner == owner {
			delete(g.tokens, key)
			return true
		}
	}
	return false
}
