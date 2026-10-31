package appearance

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/assurrussa/gocache/rcu"
	logger "github.com/assurrussa/gologger"

	"github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminactionauditrepo"
	"github.com/assurrussa/goadmin/internal/adminpreferences"
)

type brandingStore interface {
	Version(ctx context.Context) (int64, error)
	Load(ctx context.Context) (Snapshot, error)
	Save(ctx context.Context, version int64, next Snapshot) (int64, error)
}
type Option func(*Service)

func WithLogger(log logger.Logger) Option { return func(s *Service) { s.log = log } }

type Service struct {
	log         logger.Logger
	closeDone   chan struct{}
	personal    *adminpreferences.Store
	store       brandingStore
	transaction func(context.Context, func(context.Context) error) error
	audit       adminactionauditrepo.Writer
	mu          sync.RWMutex
	cache       atomic.Pointer[rcu.Cache[string, Snapshot]]
	cancel      context.CancelFunc
	closing     bool
	saves       sync.WaitGroup
}

func NewService(
	db outbox.StoragePgsqlDBEngine,
	transaction func(context.Context, func(context.Context) error) error,
	audit adminactionauditrepo.Writer,
	options ...Option,
) *Service {
	s := &Service{personal: adminpreferences.NewStore(db), store: NewBrandStore(db), transaction: transaction, audit: audit}
	for _, option := range options {
		if option != nil {
			option(s)
		}
	}
	return s
}

func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return ErrClosed
	}
	if s.cache.Load() != nil {
		s.mu.Unlock()
		return nil
	}
	owned, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	options := []rcu.Option{rcu.WithRefreshInterval(2 * time.Second)}
	if s.log != nil {
		options = append(options, rcu.WithErrorHandler(func(ctx context.Context, err error) {
			s.log.ErrorContext(ctx, "appearance branding refresh failed", logger.Error(err))
		}))
	}
	cache, err := rcu.New(owned, s.load, options...)
	if err != nil {
		cancel()
		s.mu.Unlock()
		return err
	}
	s.cache.Store(cache)
	s.mu.Unlock()
	initial, done := context.WithTimeout(ctx, time.Second)
	defer done()
	return cache.WaitInitial(initial)
}

func (s *Service) load(ctx context.Context) (map[string]Snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	v, err := s.store.Version(ctx)
	if err != nil {
		return nil, err
	}
	cache := s.cache.Load()
	if cache != nil {
		if old, ok := cache.Get("brand"); ok && old.Version == v {
			return map[string]Snapshot{"brand": old}, nil
		}
	}
	row, err := s.store.Load(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]Snapshot{"brand": row}, nil
}

func (s *Service) begin() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		return ErrClosed
	}
	s.saves.Add(1)
	return nil
}

func (s *Service) Close(ctx context.Context) error {
	s.mu.Lock()
	s.closing = true
	if s.closeDone == nil {
		s.closeDone = make(chan struct{})
		done, cache, cancel := s.closeDone, s.cache.Load(), s.cancel
		go func() {
			s.saves.Wait()
			if cancel != nil {
				cancel()
			}
			if cache != nil {
				<-cache.Done()
			}
			close(done)
		}()
	}
	done := s.closeDone
	s.mu.Unlock()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) Brand() PublicBranding {
	cache := s.cache.Load()
	if cache != nil {
		if v, ok := cache.Get("brand"); ok {
			return v.Public()
		}
	}
	return Snapshot{}.Public()
}

func (s *Service) Asset(variant string) ([]byte, string, bool) {
	cache := s.cache.Load()
	if cache == nil {
		return nil, "", false
	}
	v, ok := cache.Get("brand")
	if !ok {
		return nil, "", false
	}
	var raw string
	switch variant {
	case "light":
		raw = v.LogoLight
	case "dark":
		raw = v.LogoDark
	default:
		return nil, "", false
	}
	if raw == "" {
		return nil, "", false
	}
	return []byte(raw), fmt.Sprintf(`"%x"`, sha256.Sum256([]byte(raw))), true
}

func (s *Service) LoadPersonal(ctx context.Context, owner int64) (PersonalDocument, error) {
	if err := s.begin(); err != nil {
		return PersonalDocument{}, err
	}
	defer s.saves.Done()
	raw, v, err := s.personal.Load(ctx, owner, PersonalNamespace)
	if err != nil {
		return PersonalDocument{}, err
	}
	p := DefaultPersonal()
	if len(raw) > 0 {
		if err = json.Unmarshal(raw, &p); err != nil {
			return PersonalDocument{}, err
		}
		if err = p.Validate(); err != nil {
			return PersonalDocument{}, err
		}
	}
	return PersonalDocument{strconv.FormatInt(owner, 10), v, p}, nil
}

func (s *Service) SavePersonal(ctx context.Context, owner int64, r PersonalRequest) (PersonalDocument, error) {
	if err := s.begin(); err != nil {
		return PersonalDocument{}, err
	}
	defer s.saves.Done()
	if r.Version < 0 {
		return PersonalDocument{}, ErrInvalid
	}
	if err := r.Payload.Validate(); err != nil {
		return PersonalDocument{}, err
	}
	raw, err := json.Marshal(r.Payload)
	if err != nil {
		return PersonalDocument{}, err
	}
	v, err := s.personal.Save(ctx, owner, PersonalNamespace, r.Version, raw)
	return PersonalDocument{strconv.FormatInt(owner, 10), v, r.Payload}, err
}

func (s *Service) LoadBranding(ctx context.Context) (BrandingDocument, error) {
	if err := s.begin(); err != nil {
		return BrandingDocument{}, err
	}
	defer s.saves.Done()
	v, err := s.store.Load(ctx)
	return v.Document(), err
}

func (s *Service) SaveBranding(ctx context.Context, actor int64, r SaveBrandingRequest) (BrandingDocument, error) {
	if err := s.begin(); err != nil {
		return BrandingDocument{}, err
	}
	defer s.saves.Done()
	if actor <= 0 || r.Version < 0 {
		return BrandingDocument{}, ErrInvalid
	}
	if s.transaction == nil || s.audit == nil {
		return BrandingDocument{}, errors.New("branding transaction and audit required")
	}
	var next Snapshot
	err := s.transaction(ctx, func(txCtx context.Context) error {
		old, err := s.store.Load(txCtx)
		if err != nil {
			return err
		}
		if old.Version != r.Version {
			return adminpreferences.ErrConflict
		}
		next, err = r.apply(old)
		if err != nil {
			return err
		}
		next.Version, err = s.store.Save(txCtx, r.Version, next)
		if err != nil {
			return err
		}
		return s.audit.RecordAdminAction(txCtx, adminactionauditrepo.Record{
			ActorAdminID: actor, Action: "branding.updated",
			TargetType: "branding", TargetID: "installation",
		})
	})
	if err != nil {
		return BrandingDocument{}, err
	}
	result := next.Document()
	cache := s.cache.Load()
	if cache == nil {
		result.Pending = true
		return result, nil
	}
	refresh, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if cache.Refresh(refresh) != nil {
		result.Pending = true
	}
	return result, nil
}
