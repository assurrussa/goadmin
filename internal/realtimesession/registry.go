// Package realtimesession fences one runtime's explicit session logout against
// WebSocket authentication and asynchronous upgrade. It is not a revocation bus.
package realtimesession

import (
	"strings"
	"sync"
	"time"

	"github.com/assurrussa/goauth"
)

// Key is canonical, verified server identity. Browser IDs rotate; projection
// UUIDs address user events and cannot distinguish independent logins.
type Key struct {
	SubjectID     goauth.SubjectID
	AuthSessionID string
}

func (k Key) valid() bool {
	return !k.SubjectID.IsZero() && k.AuthSessionID != "" && strings.TrimSpace(k.AuthSessionID) == k.AuthSessionID
}

// Registry's zero value is ready to use. It never owns a stream or auth client.
type Registry struct {
	mu      sync.Mutex
	epoch   uint64
	pending map[*Admission]uint64
	revoked map[Key]uint64
	active  map[Key]map[*Lease]struct{}
}

// Admission is captured before authentication, when its canonical key is unknown.
type Admission struct {
	registry *Registry
	epoch    uint64
	finished bool
	timer    *time.Timer
}

// Lease covers both a pending hijack callback and an attached socket. All fields
// are protected by registry.mu; terminal leases never become usable again.
type Lease struct {
	registry    *Registry
	key         Key
	finished    bool
	closeSocket func()
	timer       *time.Timer
}

func (r *Registry) Begin(timeout time.Duration) *Admission {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := &Admission{registry: r, epoch: r.epoch}
	if r.pending == nil {
		r.pending = make(map[*Admission]uint64)
	}
	r.pending[a] = a.epoch
	a.timer = time.AfterFunc(timeout, a.Cancel)
	return a
}

func (a *Admission) Cancel() {
	if a == nil {
		return
	}
	r := a.registry
	r.mu.Lock()
	defer r.mu.Unlock()
	a.finishLocked()
}

func (a *Admission) finishLocked() {
	if a.finished {
		return
	}
	a.finished = true
	a.timer.Stop()
	delete(a.registry.pending, a)
	a.registry.pruneLocked()
}

// Bind consumes admission after authentication and before initiating HTTP 101.
func (a *Admission) Bind(key Key, handoffTimeout time.Duration) *Lease {
	if a == nil {
		return nil
	}
	r := a.registry
	r.mu.Lock()
	defer r.mu.Unlock()
	if a.finished {
		return nil
	}
	allowed := key.valid() && r.revoked[key] <= a.epoch
	a.finishLocked()
	if !allowed {
		return nil
	}
	l := &Lease{registry: r, key: key}
	if r.active == nil {
		r.active = make(map[Key]map[*Lease]struct{})
	}
	if r.active[key] == nil {
		r.active[key] = make(map[*Lease]struct{})
	}
	r.active[key][l] = struct{}{}
	l.timer = time.AfterFunc(handoffTimeout, l.expirePending)
	return l
}

func (l *Lease) Attach(closeSocket func()) bool {
	if l == nil || closeSocket == nil {
		return false
	}
	r := l.registry
	r.mu.Lock()
	defer r.mu.Unlock()
	if l.finished || l.closeSocket != nil {
		return false
	}
	l.closeSocket = closeSocket
	l.timer.Stop()
	return true
}

func (l *Lease) expirePending() {
	r := l.registry
	r.mu.Lock()
	defer r.mu.Unlock()
	if l.closeSocket == nil {
		l.releaseLocked()
	}
}

func (l *Lease) Release() {
	if l == nil {
		return
	}
	r := l.registry
	r.mu.Lock()
	defer r.mu.Unlock()
	l.releaseLocked()
}

func (l *Lease) releaseLocked() {
	if l.finished {
		return
	}
	l.finished = true
	l.timer.Stop()
	l.closeSocket = nil
	delete(l.registry.active[l.key], l)
	if len(l.registry.active[l.key]) == 0 {
		delete(l.registry.active, l.key)
	}
}

// Revoke is called only after successful canonical logout. Copy close callbacks
// under the lock, then execute them outside it. Future authentication consults
// canonical authority; tombstones are needed only for older unbound admissions.
func (r *Registry) Revoke(key Key) {
	if !key.valid() {
		return
	}
	r.mu.Lock()
	r.epoch++
	if r.revoked == nil {
		r.revoked = make(map[Key]uint64)
	}
	r.revoked[key] = r.epoch
	var closes []func()
	for l := range r.active[key] {
		if l.closeSocket != nil {
			closes = append(closes, l.closeSocket)
		}
		l.releaseLocked()
	}
	r.pruneLocked()
	r.mu.Unlock()
	for _, closeSocket := range closes {
		closeSocket()
	}
}

func (r *Registry) pruneLocked() {
	oldest := r.epoch
	for _, epoch := range r.pending {
		if epoch < oldest {
			oldest = epoch
		}
	}
	for key, epoch := range r.revoked {
		if epoch <= oldest {
			delete(r.revoked, key)
		}
	}
}
