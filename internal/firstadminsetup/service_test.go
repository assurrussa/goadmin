package firstadminsetup //nolint:testpackage // uses a focused in-package fake for the unexported token encoding contract

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestServiceStoresOnlyHashAndConsumesOnce(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 10, 12, 0, 0, 0, time.UTC)
	store := &memoryStore{}
	service, err := New(
		store,
		WithClock(func() time.Time { return now }),
		WithRandom(strings.NewReader(strings.Repeat("s", tokenBytes))),
	)
	require.NoError(t, err)

	token, err := service.Issue(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.NotContains(t, string(store.hash[:]), token)
	require.Equal(t, sha256.Sum256([]byte(token)), store.hash)
	require.Equal(t, now.Add(DefaultTTL), store.expiresAt)

	require.NoError(t, service.Validate(context.Background(), token))
	require.NoError(t, service.Consume(context.Background(), token))
	require.ErrorIs(t, service.Consume(context.Background(), token), ErrInvalidSetupToken)
}

func TestServiceRejectsExpiredAndMalformedTokens(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 10, 12, 0, 0, 0, time.UTC)
	store := &memoryStore{}
	service, err := New(
		store,
		WithTTL(time.Minute),
		WithClock(func() time.Time { return now }),
		WithRandom(strings.NewReader(strings.Repeat("e", tokenBytes))),
	)
	require.NoError(t, err)

	token, err := service.Issue(context.Background())
	require.NoError(t, err)
	now = now.Add(time.Minute)

	require.ErrorIs(t, service.Validate(context.Background(), token), ErrInvalidSetupToken)
	require.ErrorIs(t, service.Consume(context.Background(), token), ErrInvalidSetupToken)
	require.ErrorIs(t, service.Validate(context.Background(), "not-a-token"), ErrInvalidSetupToken)
}

func TestServiceSupportsStaticTokenWithoutRetainingRawValue(t *testing.T) {
	t.Parallel()

	raw := strings.Repeat("static-secret-", 3)
	store := &memoryStore{}
	service, err := New(store, WithStaticTokenHash(sha256.Sum256([]byte(raw))))
	require.NoError(t, err)
	require.True(t, service.StaticConfigured())

	require.NoError(t, service.Validate(context.Background(), raw))
	require.ErrorIs(t, service.Validate(context.Background(), raw+"wrong"), ErrInvalidSetupToken)
	require.NoError(t, service.Consume(context.Background(), raw))
	require.Equal(t, 1, store.staticConsumeCalls)

	store.closed = true
	require.ErrorIs(t, service.Consume(context.Background(), raw), ErrClosed)
}

func TestServiceWithoutStaticTokenReportsDisabled(t *testing.T) {
	t.Parallel()

	service, err := New(&memoryStore{})
	require.NoError(t, err)
	require.False(t, service.StaticConfigured())
}

type memoryStore struct {
	hash               [sha256.Size]byte
	expiresAt          time.Time
	consumedAt         time.Time
	revokedAt          time.Time
	closed             bool
	staticConsumeCalls int
}

func (s *memoryStore) ConsumeStatic(_ context.Context) error {
	if s.closed {
		return ErrClosed
	}
	s.staticConsumeCalls++

	return nil
}

func (s *memoryStore) Issue(
	_ context.Context,
	hash [sha256.Size]byte,
	issuedAt time.Time,
	ttl time.Duration,
) error {
	if s.closed {
		return ErrClosed
	}
	s.hash = hash
	s.expiresAt = issuedAt.Add(ttl)
	s.consumedAt = time.Time{}
	s.revokedAt = time.Time{}

	return nil
}

func (s *memoryStore) Validate(
	_ context.Context,
	hash [sha256.Size]byte,
	now time.Time,
) error {
	if hash != s.hash || !now.Before(s.expiresAt) || !s.consumedAt.IsZero() || !s.revokedAt.IsZero() {
		return ErrInvalidSetupToken
	}

	return nil
}

func (s *memoryStore) Consume(
	ctx context.Context,
	hash [sha256.Size]byte,
	now time.Time,
) error {
	if err := s.Validate(ctx, hash, now); err != nil {
		return err
	}
	s.consumedAt = now

	return nil
}

func (s *memoryStore) Revoke(
	_ context.Context,
	hash [sha256.Size]byte,
	now time.Time,
) error {
	if hash != s.hash {
		return errors.New("unknown hash")
	}
	s.revokedAt = now

	return nil
}
