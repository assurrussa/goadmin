package sessionredis_test

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
	redisv9 "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/assurrussa/goadmin/config"
	"github.com/assurrussa/goadmin/infrastructure/core/session/sessionredis"
	redismocks "github.com/assurrussa/goadmin/infrastructure/redis/testsupport"
)

func TestCreateSessionStore(t *testing.T) {
	t.Run("not production", func(t *testing.T) {
		cfg := config.Config{
			BaseDomain:         "test.local",
			BaseDomainURL:      "https://test.local",
			Env:                "local",
			Domain:             "admin.test.local",
			DomainURL:          "https://admin.test.local",
			SessionTTL:         24 * time.Hour,
			SessionInactiveTTL: 30 * time.Minute,
		}
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		mockRedis := redismocks.NewMockClientContract(mockCtrl)
		handler, store := sessionredis.CreateAdminSessionStore(mockRedis, cfg)
		require.NotNil(t, handler)
		require.IsType(t, &session.Store{}, store)
		conf := store.Config
		require.NotNil(t, conf.Extractor)
		require.True(t, conf.CookieHTTPOnly)
		require.False(t, conf.CookieSecure)
		require.Equal(t, 30*time.Minute, conf.IdleTimeout)
		require.Equal(t, 24*time.Hour, conf.AbsoluteTimeout)
		require.Equal(t, "Lax", conf.CookieSameSite)
		require.Equal(t, "admin.test.local", conf.CookieDomain)
		require.True(t, conf.CookieSessionOnly)
	})

	t.Run("production", func(t *testing.T) {
		cfg := config.Config{
			BaseDomain:         "test.local",
			BaseDomainURL:      "https://test.local",
			Env:                "production",
			Domain:             "admin.test.local",
			DomainURL:          "https://admin.test.local",
			SessionTTL:         24 * time.Hour,
			SessionInactiveTTL: 30 * time.Minute,
		}
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		mockRedis := redismocks.NewMockClientContract(mockCtrl)
		handler, store := sessionredis.CreateAdminSessionStore(mockRedis, cfg)
		require.NotNil(t, handler)
		conf := store.Config
		require.Empty(t, conf.CookieDomain)
		require.Equal(t, "__Host-session_id", conf.Extractor.Key)
		require.Equal(t, "/", conf.CookiePath)
		require.True(t, conf.CookieHTTPOnly)
		require.True(t, conf.CookieSecure)
	})
}

func TestSessionStorageUsesNamespaceAndBorrowsClient(t *testing.T) {
	ctx := t.Context()
	mockRedis := redismocks.NewMockClientContract(gomock.NewController(t))
	_, store := sessionredis.CreateAdminSessionStore(mockRedis, config.Config{})
	storage := store.Storage

	mockRedis.EXPECT().Set(ctx, "goadmin:session:session-1", []byte("value"), time.Minute).
		Return(redisv9.NewStatusResult("OK", nil))
	require.NoError(t, storage.SetWithContext(ctx, "session-1", []byte("value"), time.Minute))

	payload := encodeSession(t, map[any]any{"actor": "subject-1"})
	mockRedis.EXPECT().Get(ctx, "goadmin:session:session-1").
		Return(redisv9.NewStringResult(string(payload), nil))
	got, err := storage.GetWithContext(ctx, "session-1")
	require.NoError(t, err)
	require.Equal(t, payload, got)

	mockRedis.EXPECT().Del(ctx, "goadmin:session:session-1").
		Return(redisv9.NewIntResult(1, nil))
	require.NoError(t, storage.DeleteWithContext(ctx, "session-1"))

	// Fiber closes its storage; the host still owns the shared Redis client.
	require.NoError(t, storage.Close())
}

func TestSessionStorageResetOnlyScansSessionNamespace(t *testing.T) {
	ctx := t.Context()
	mockRedis := redismocks.NewMockClientContract(gomock.NewController(t))
	_, store := sessionredis.CreateAdminSessionStore(mockRedis, config.Config{})

	gomock.InOrder(
		mockRedis.EXPECT().Scan(ctx, uint64(0), "goadmin:session:*", int64(100)).
			Return(redisv9.NewScanCmdResult([]string{"goadmin:session:one", "goadmin:session:two"}, 7, nil)),
		mockRedis.EXPECT().Del(ctx, "goadmin:session:one", "goadmin:session:two").
			Return(redisv9.NewIntResult(2, nil)),
		mockRedis.EXPECT().Scan(ctx, uint64(7), "goadmin:session:*", int64(100)).
			Return(redisv9.NewScanCmdResult(nil, 0, nil)),
	)
	require.NoError(t, store.Reset(ctx))
}

func TestSessionStorageResetReturnsScanError(t *testing.T) {
	ctx := t.Context()
	mockRedis := redismocks.NewMockClientContract(gomock.NewController(t))
	_, store := sessionredis.CreateAdminSessionStore(mockRedis, config.Config{})
	want := errors.New("scan failed")
	mockRedis.EXPECT().Scan(ctx, uint64(0), "goadmin:session:*", int64(100)).
		Return(redisv9.NewScanCmdResult(nil, 0, want))
	require.ErrorIs(t, store.Reset(ctx), want)
}

func TestSessionStorageResetEnumeratesShards(t *testing.T) {
	ctx := t.Context()
	mockRedis := redismocks.NewMockClientShardContract(gomock.NewController(t))
	_, store := sessionredis.CreateAdminSessionStore(mockRedis, config.Config{})
	want := errors.New("shard unavailable")
	mockRedis.EXPECT().ForEachShard(ctx, gomock.Any()).Return(want)
	require.ErrorIs(t, store.Reset(ctx), want)
}

func encodeSession(t *testing.T, value any) []byte {
	t.Helper()
	var data bytes.Buffer
	require.NoError(t, gob.NewEncoder(&data).Encode(value))
	return data.Bytes()
}

type legacySessionActor struct{ Subject string }

func TestSessionStorageRejectsIncompatibleGob(t *testing.T) {
	gob.RegisterName("legacy-session-user", legacySessionActor{})
	unknownType := bytes.ReplaceAll(
		encodeSession(t, map[any]any{"actor": legacySessionActor{Subject: "old-admin"}}),
		[]byte("legacy-session-user"), []byte("absent-session-user"),
	)
	var decoded map[any]any
	require.ErrorContains(t, gob.NewDecoder(bytes.NewReader(unknownType)).Decode(&decoded), "name not registered")
	for _, payload := range [][]byte{
		unknownType,
		[]byte("obsolete serialized session"),
		encodeSession(t, struct{ LegacyActor string }{LegacyActor: "old-admin"}),
	} {
		ctx := t.Context()
		client := redismocks.NewMockClientContract(gomock.NewController(t))
		_, store := sessionredis.CreateAdminSessionStore(client, config.Config{})
		gomock.InOrder(
			client.EXPECT().Get(ctx, "goadmin:session:old").Return(redisv9.NewStringResult(string(payload), nil)),
			client.EXPECT().Del(ctx, "goadmin:session:old").Return(redisv9.NewIntResult(1, nil)),
		)
		got, err := store.Storage.GetWithContext(ctx, "old")
		require.NoError(t, err)
		require.Nil(t, got)
	}
}

func TestSessionStorageReadAndInvalidRecordDeleteErrorsPropagate(t *testing.T) {
	for _, readFails := range []bool{true, false} {
		client := redismocks.NewMockClientContract(gomock.NewController(t))
		_, store := sessionredis.CreateAdminSessionStore(client, config.Config{})
		want := errors.New("redis unavailable")
		if readFails {
			client.EXPECT().Get(t.Context(), "goadmin:session:old").Return(redisv9.NewStringResult("", want))
		} else {
			client.EXPECT().Get(t.Context(), "goadmin:session:old").Return(redisv9.NewStringResult("invalid gob", nil))
			client.EXPECT().Del(t.Context(), "goadmin:session:old").Return(redisv9.NewIntResult(0, want))
		}
		got, err := store.Storage.GetWithContext(t.Context(), "old")
		require.ErrorIs(t, err, want)
		require.Nil(t, got)
	}
}

func TestIncompatibleBrowserSessionBecomesAnonymousWithFreshID(t *testing.T) {
	client := redismocks.NewMockClientContract(gomock.NewController(t))
	middleware, _ := sessionredis.CreateAdminSessionStore(client, config.Config{})
	client.EXPECT().Get(gomock.Any(), "goadmin:session:old-cookie").Return(redisv9.NewStringResult("unknown gob", nil))
	client.EXPECT().Del(gomock.Any(), "goadmin:session:old-cookie").Return(redisv9.NewIntResult(1, nil))
	var newID string
	client.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, key string, value any, _ time.Duration) *redisv9.StatusCmd {
			require.Equal(t, "goadmin:session:"+newID, key)
			var decoded map[any]any
			payload, ok := value.([]byte)
			require.True(t, ok)
			require.NoError(t, gob.NewDecoder(bytes.NewReader(payload)).Decode(&decoded))
			require.NotContains(t, decoded, "actor")
			return redisv9.NewStatusResult("OK", nil)
		},
	)
	app := fiber.New()
	app.Use(middleware)
	app.Get("/", func(c fiber.Ctx) error {
		current := session.FromContext(c)
		require.Nil(t, current.Get("actor"))
		newID = current.Session.ID()
		require.NotEmpty(t, newID)
		require.NotEqual(t, "old-cookie", newID)
		return c.SendStatus(http.StatusOK)
	})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.test/", nil)
	req.AddCookie(&http.Cookie{
		Name: "session_id", Value: "old-cookie", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	response, err := app.Test(req)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	var sessionCookie *http.Cookie
	for _, cookie := range response.Cookies() {
		if cookie.Name == "session_id" {
			sessionCookie = cookie
		}
	}
	require.NotNil(t, sessionCookie)
	require.Equal(t, newID, sessionCookie.Value)
}
