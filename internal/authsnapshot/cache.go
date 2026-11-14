// Package authsnapshot holds an immutable admin authorization view. It does not
// cache authentication, session revocation, security versions, or MFA state.
package authsnapshot

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/rbac"
	"github.com/assurrussa/gocache/rcu"
)

var ErrUnavailable = errors.New("admin authorization snapshot unavailable")

// Data must be loaded from one consistent database snapshot. Members are
// explicit admin memberships; having a role alone never creates membership.
// The loader transfers ownership of its data and must not mutate it afterward.
type Data struct {
	AdminIDs map[int64]goauth.SubjectID
	Members  []goauth.SubjectID
	RBAC     rbac.Snapshot
}

type Loader func(context.Context) (Data, error)

type Config struct {
	RefreshInterval time.Duration
	MaxAge          time.Duration
	LoadTimeout     time.Duration
	OnError         rcu.ErrorHandler
}

type state struct {
	adminIDs        map[int64]goauth.SubjectID
	rolePermissions map[int64][]rbac.Permission
	generation      uint64
	loadedAt        time.Time
	members         map[goauth.SubjectID]struct{}
	roles           map[goauth.SubjectID][]rbac.Role
	permissions     map[goauth.SubjectID]map[rbac.PermissionKey]struct{}
}

type Cache struct {
	data       *rcu.Cache[int, *state]
	generation atomic.Uint64
	done       <-chan struct{}
	cancel     context.CancelFunc
	maxAge     time.Duration
	now        func() time.Time
}

// New waits for initial load. Disabled mode is represented by not constructing
// a Cache, never by treating an unready cache as an authoritative empty store.
func New(ctx context.Context, loader Loader, cfg Config) (*Cache, error) {
	if ctx == nil || loader == nil {
		return nil, errors.New("authorization snapshot context and loader required")
	}
	if cfg.RefreshInterval == 0 {
		cfg.RefreshInterval = 30 * time.Second
	}
	if cfg.MaxAge == 0 {
		cfg.MaxAge = 2 * time.Minute
	}
	if cfg.LoadTimeout == 0 {
		cfg.LoadTimeout = 5 * time.Second
	}
	if cfg.RefreshInterval < 0 || cfg.MaxAge <= cfg.RefreshInterval || cfg.LoadTimeout < 0 || cfg.LoadTimeout >= cfg.MaxAge {
		return nil, errors.New("invalid authorization snapshot refresh, age or load timeout")
	}
	owned, cancel := context.WithCancel(ctx)
	c := &Cache{done: owned.Done(), cancel: cancel, maxAge: cfg.MaxAge, now: time.Now}
	options := []rcu.Option{rcu.WithRefreshInterval(cfg.RefreshInterval)}
	if cfg.OnError != nil {
		options = append(options, rcu.WithErrorHandler(cfg.OnError))
	}
	cache, err := rcu.New(owned, func(ctx context.Context) (map[int]*state, error) {
		generation := c.generation.Load()
		started := c.now()
		loadCtx, done := context.WithTimeout(ctx, cfg.LoadTimeout)
		defer done()
		data, err := loader(loadCtx)
		if err != nil {
			return nil, err
		}
		if err := loadCtx.Err(); err != nil {
			return nil, err
		}
		next := build(data)
		next.generation, next.loadedAt = generation, started
		return map[int]*state{0: next}, nil
	}, options...)
	if err != nil {
		cancel()
		return nil, err
	}
	c.data = cache
	if err := cache.WaitInitial(ctx); err != nil {
		cancel()
		<-cache.Done()
		return nil, err
	}
	return c, nil
}

func build(data Data) *state {
	s := &state{
		adminIDs:        make(map[int64]goauth.SubjectID),
		rolePermissions: make(map[int64][]rbac.Permission),
		members:         make(map[goauth.SubjectID]struct{}),
		roles:           make(map[goauth.SubjectID][]rbac.Role),
		permissions:     make(map[goauth.SubjectID]map[rbac.PermissionKey]struct{}),
	}
	for _, id := range data.Members {
		s.members[id] = struct{}{}
	}
	for id, subject := range data.AdminIDs {
		if _, ok := s.members[subject]; ok {
			s.adminIDs[id] = subject
		}
	}
	roles := make(map[int64]rbac.Role, len(data.RBAC.Roles))
	for _, r := range data.RBAC.Roles {
		roles[r.ID] = r
	}
	permissions := make(map[int64]rbac.PermissionKey, len(data.RBAC.Permissions))
	for _, p := range data.RBAC.Permissions {
		permissions[p.ID] = p.Key
	}
	fullPermissions := make(map[int64]rbac.Permission)
	for _, p := range data.RBAC.Permissions {
		fullPermissions[p.ID] = p
	}
	byRole := make(map[int64][]rbac.PermissionKey)
	for _, p := range data.RBAC.RolePermissions {
		if key, ok := permissions[p.PermissionID]; ok {
			byRole[p.RoleID] = append(byRole[p.RoleID], key)
			s.rolePermissions[p.RoleID] = append(s.rolePermissions[p.RoleID], fullPermissions[p.PermissionID])
		}
	}
	for _, sr := range data.RBAC.SubjectRoles {
		if _, ok := s.members[sr.SubjectID]; !ok {
			continue
		}
		r, ok := roles[sr.RoleID]
		if !ok {
			continue
		}
		s.roles[sr.SubjectID] = append(s.roles[sr.SubjectID], r)
		if s.permissions[sr.SubjectID] == nil {
			s.permissions[sr.SubjectID] = make(map[rbac.PermissionKey]struct{})
		}
		for _, key := range byRole[r.ID] {
			s.permissions[sr.SubjectID][key] = struct{}{}
		}
	}
	return s
}

// View pins one generation for a complete authorization decision. Do not retain
// it beyond a request or use it for transactional management invariants.
type View struct{ state *state }

func (c *Cache) View(ctx context.Context) (View, error) {
	if c == nil || ctx == nil {
		return View{}, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return View{}, err
	}
	select {
	case <-c.done:
		return View{}, ErrUnavailable
	default:
	}
	s, ok := c.data.Get(0)
	if !ok || s.generation != c.generation.Load() || c.now().Sub(s.loadedAt) >= c.maxAge {
		return View{}, ErrUnavailable
	}
	return View{state: s}, nil
}

func (v View) Member(id goauth.SubjectID) bool {
	if v.state == nil {
		return false
	}
	_, ok := v.state.members[id]
	return ok
}

func (v View) Roles(id goauth.SubjectID) []rbac.Role {
	if v.state == nil {
		return nil
	}
	return slices.Clone(v.state.roles[id])
}

func (v View) Allows(id goauth.SubjectID, key rbac.PermissionKey) bool {
	if v.state == nil {
		return false
	}
	_, ok := v.state.permissions[id][key]
	return ok
}

// HasPermission implements GoAuth's cache contract. A valid negative result is
// authoritative (found=true); unavailability is an error, not read-through SQL.
func (c *Cache) HasPermission(
	ctx context.Context, id goauth.SubjectID, key rbac.PermissionKey,
) (allowed bool, found bool, err error) {
	v, err := c.View(ctx)
	if err != nil {
		return false, false, err
	}
	return v.Allows(id, key), true, nil
}

// Invalidate immediately fences an in-flight old load, then schedules a rebuild.
// Call only AFTER commit, not inside a transaction that might still roll back.
func (c *Cache) Invalidate(ctx context.Context) error {
	if c == nil || ctx == nil {
		return ErrUnavailable
	}
	// Invalidate even when the caller has already been cancelled: never retain a
	// known stale grant simply because the originating HTTP request disconnected.
	c.NotifyChange()
	return ctx.Err()
}

func (c *Cache) Close() {
	if c == nil {
		return
	}
	c.cancel()
	<-c.data.Done()
}

func (v View) AdminSubject(id int64) (goauth.SubjectID, bool) {
	if v.state == nil {
		return goauth.SubjectID{}, false
	}
	subject, ok := v.state.adminIDs[id]
	return subject, ok
}

func (v View) RolePermissions(id int64) []rbac.Permission {
	if v.state == nil {
		return nil
	}
	return slices.Clone(v.state.rolePermissions[id])
}

// NotifyChange fences committed changes without inheriting request cancellation.
func (c *Cache) NotifyChange() {
	if c == nil {
		return
	}
	c.generation.Add(1)
	c.data.Notify()
}
