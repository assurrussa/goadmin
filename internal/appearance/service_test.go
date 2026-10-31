package appearance //nolint:testpackage // Injects private branding stores and observes owned cache lifecycle.

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminactionauditrepo"
)

type memoryBrand struct {
	mu       sync.Mutex
	snapshot Snapshot
	fail     bool
	loads    int
}

func (m *memoryBrand) Version(ctx context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return 0, errors.New("unavailable")
	}
	return m.snapshot.Version, ctx.Err()
}

func (m *memoryBrand) Load(ctx context.Context) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loads++
	return m.snapshot, ctx.Err()
}

func (m *memoryBrand) Save(ctx context.Context, _ int64, s Snapshot) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.snapshot = s
	return s.Version, ctx.Err()
}

type auditFunc func(context.Context, adminactionauditrepo.Record) error

func (f auditFunc) RecordAdminAction(c context.Context, r adminactionauditrepo.Record) error {
	return f(c, r)
}

func TestNormalizationAndInputBounds(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	img.Set(0, 0, color.White)
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, nil))
	normalized, err := NormalizeLogo(base64.StdEncoding.EncodeToString(buf.Bytes()))
	require.NoError(t, err)
	cfg, err := png.DecodeConfig(bytes.NewReader(normalized))
	require.NoError(t, err)
	require.Equal(t, 3, cfg.Width)
	for _, bad := range []string{"aGVsbG8=", "<svg/>", base64.StdEncoding.EncodeToString(make([]byte, MaxLogoBytes+1))} {
		_, err = NormalizeLogo(bad)
		require.ErrorIs(t, err, ErrInvalid)
	}
	buf.Reset()
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1025, 1))))
	_, err = NormalizeLogo(base64.StdEncoding.EncodeToString(buf.Bytes()))
	require.ErrorIs(t, err, ErrInvalid)
	for _, p := range []PersonalPayload{
		{1, "other", "ru", "default"},
		{1, "system", "xx", "default"},
		{1, "dark", "en", "huge"},
		{0, "dark", "en", "compact"},
	} {
		require.ErrorIs(t, p.Validate(), ErrInvalid)
	}
}

func TestCacheHoldsLastGoodAndOwnsImmutableBytes(t *testing.T) {
	ctx := context.Background()
	store := &memoryBrand{snapshot: Snapshot{Version: 1, Name: "old", LogoLight: "PNG"}}
	s := &Service{store: store}
	require.NoError(t, s.Start(ctx))
	t.Cleanup(func() { require.NoError(t, s.Close(ctx)) })
	raw, _, ok := s.Asset("light")
	require.True(t, ok)
	raw[0] = 'X'
	again, _, _ := s.Asset("light")
	require.Equal(t, "PNG", string(again))
	require.NoError(t, s.cache.Load().Refresh(ctx))
	store.mu.Lock()
	require.Equal(t, 1, store.loads)
	store.fail = true
	store.mu.Unlock()
	require.Error(t, s.cache.Load().Refresh(ctx))
	require.Equal(t, "old", s.Brand().Name)
	store.mu.Lock()
	store.fail = false
	store.snapshot = Snapshot{Version: 2, Name: "new"}
	store.mu.Unlock()
	require.NoError(t, s.cache.Load().Refresh(ctx))
	require.Equal(t, "new", s.Brand().Name)
}

func TestCommittedWriteRemainsSuccessfulWhenRefreshFails(t *testing.T) {
	store := &memoryBrand{}
	s := &Service{store: store, audit: auditFunc(func(context.Context, adminactionauditrepo.Record) error {
		store.mu.Lock()
		store.fail = true
		store.mu.Unlock()
		return nil
	}), transaction: func(c context.Context, f func(context.Context) error) error { return f(c) }}
	ctx := context.Background()
	require.NoError(t, s.Start(ctx))
	defer func() { require.NoError(t, s.Close(ctx)) }()
	doc, err := s.SaveBranding(ctx, 1, SaveBrandingRequest{Version: 0, Name: "Saved"})
	require.NoError(t, err)
	require.True(t, doc.Pending)
	require.Equal(t, "Saved", doc.Payload.Name)
	require.EqualValues(t, 1, doc.Version)
}

func TestConcurrentReadRefreshAndShutdown(t *testing.T) {
	s := &Service{store: &memoryBrand{}}
	ctx := context.Background()
	require.NoError(t, s.Start(ctx))
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 30 {
				_ = s.Brand()
				_, _, _ = s.Asset("dark")
				_ = s.cache.Load().Refresh(ctx)
			}
		})
	}
	wg.Wait()
	shutdown, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	require.NoError(t, s.Close(shutdown))
	select {
	case <-s.cache.Load().Done():
	default:
		t.Fatal("worker still running")
	}
	require.ErrorIs(t, s.Start(ctx), ErrClosed)
	_, err := s.SaveBranding(ctx, 1, SaveBrandingRequest{})
	require.ErrorIs(t, err, ErrClosed)
}

func TestCloseDrainsAcceptedSaveAndRejectsNewSaves(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	s := &Service{
		store:       &memoryBrand{},
		transaction: func(c context.Context, f func(context.Context) error) error { return f(c) },
		audit: auditFunc(func(context.Context, adminactionauditrepo.Record) error {
			close(entered)
			<-release
			return nil
		}),
	}
	ctx := context.Background()
	require.NoError(t, s.Start(ctx))
	saved := make(chan error, 1)
	go func() { _, err := s.SaveBranding(ctx, 1, SaveBrandingRequest{Name: "Accepted"}); saved <- err }()
	<-entered
	timeout, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, s.Close(timeout), context.DeadlineExceeded)
	_, err := s.SaveBranding(ctx, 1, SaveBrandingRequest{Name: "Rejected"})
	require.ErrorIs(t, err, ErrClosed)
	select {
	case <-s.cache.Load().Done():
		t.Fatal("cache stopped before accepted save drained")
	default:
	}
	close(release)
	require.NoError(t, <-saved)
	require.NoError(t, s.Close(ctx))
	require.Equal(t, "Accepted", s.Brand().Name)
}

type blockingBrandRead struct {
	memoryBrand
	entered chan struct{}
	release chan struct{}
}

func (b *blockingBrandRead) Load(ctx context.Context) (Snapshot, error) {
	close(b.entered)
	select {
	case <-b.release:
		return Snapshot{Name: "accepted"}, nil
	case <-ctx.Done():
		return Snapshot{}, ctx.Err()
	}
}

func TestCloseDrainsAcceptedReadAndRejectsNewReads(t *testing.T) {
	store := &blockingBrandRead{entered: make(chan struct{}), release: make(chan struct{})}
	s := &Service{store: store}
	ctx := context.Background()
	read := make(chan error, 1)
	go func() { _, err := s.LoadBranding(ctx); read <- err }()
	<-store.entered
	timeout, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, s.Close(timeout), context.DeadlineExceeded)
	_, err := s.LoadBranding(ctx)
	require.ErrorIs(t, err, ErrClosed)
	_, err = s.LoadPersonal(ctx, 1)
	require.ErrorIs(t, err, ErrClosed)
	close(store.release)
	require.NoError(t, <-read)
	require.NoError(t, s.Close(ctx))
}
