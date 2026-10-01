package firstadminsetup

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	DefaultTTL = 15 * time.Minute
	tokenBytes = 32
)

var (
	ErrClosed            = errors.New("first admin setup is closed")
	ErrInvalidSetupToken = errors.New("first admin setup token is invalid, expired, or already used")
)

type Store interface {
	Issue(
		ctx context.Context,
		tokenHash [sha256.Size]byte,
		issuedAt time.Time,
		ttl time.Duration,
	) error
	Validate(ctx context.Context, tokenHash [sha256.Size]byte, now time.Time) error
	Consume(ctx context.Context, tokenHash [sha256.Size]byte, now time.Time) error
	ConsumeStatic(ctx context.Context) error
	Revoke(ctx context.Context, tokenHash [sha256.Size]byte, now time.Time) error
}

type Service struct {
	store                 Store
	ttl                   time.Duration
	now                   func() time.Time
	random                io.Reader
	staticTokenHash       [sha256.Size]byte
	staticTokenConfigured bool
}

type Option func(*Service)

func WithTTL(ttl time.Duration) Option {
	return func(service *Service) {
		service.ttl = ttl
	}
}

func WithClock(now func() time.Time) Option {
	return func(service *Service) {
		service.now = now
	}
}

func WithRandom(random io.Reader) Option {
	return func(service *Service) {
		service.random = random
	}
}

func WithStaticTokenHash(hash [sha256.Size]byte) Option {
	return func(service *Service) {
		service.staticTokenHash = hash
		service.staticTokenConfigured = true
	}
}

func New(store Store, opts ...Option) (*Service, error) {
	if store == nil {
		return nil, errors.New("first admin setup store is required")
	}

	service := &Service{
		store:  store,
		ttl:    DefaultTTL,
		now:    time.Now,
		random: rand.Reader,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	if service.ttl <= 0 {
		return nil, errors.New("first admin setup token ttl must be positive")
	}
	if service.now == nil {
		return nil, errors.New("first admin setup clock is required")
	}
	if service.random == nil {
		return nil, errors.New("first admin setup random source is required")
	}

	return service, nil
}

func (s *Service) Issue(ctx context.Context) (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := io.ReadFull(s.random, raw); err != nil {
		return "", fmt.Errorf("generate first admin setup token: %w", err)
	}

	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	if err := s.store.Issue(ctx, hash, s.now(), s.ttl); err != nil {
		return "", err
	}

	return token, nil
}

func (s *Service) Validate(ctx context.Context, token string) error {
	if s.matchesStaticToken(token) {
		return nil
	}

	hash, err := tokenHash(token)
	if err != nil {
		return err
	}

	return s.store.Validate(ctx, hash, s.now())
}

func (s *Service) Consume(ctx context.Context, token string) error {
	if s.matchesStaticToken(token) {
		return s.store.ConsumeStatic(ctx)
	}

	hash, err := tokenHash(token)
	if err != nil {
		return err
	}

	return s.store.Consume(ctx, hash, s.now())
}

func (s *Service) StaticConfigured() bool {
	return s.staticTokenConfigured
}

func (s *Service) matchesStaticToken(token string) bool {
	if !s.staticTokenConfigured {
		return false
	}

	hash := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return subtle.ConstantTimeCompare(hash[:], s.staticTokenHash[:]) == 1
}

func (s *Service) Revoke(ctx context.Context, token string) error {
	hash, err := tokenHash(token)
	if err != nil {
		return err
	}

	return s.store.Revoke(ctx, hash, s.now())
}

func tokenHash(token string) ([sha256.Size]byte, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return [sha256.Size]byte{}, ErrInvalidSetupToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != tokenBytes {
		return [sha256.Size]byte{}, ErrInvalidSetupToken
	}

	return sha256.Sum256([]byte(token)), nil
}
