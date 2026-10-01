//go:build integration

package sessionpgsql_test

import (
	"bytes"
	"context"
	"encoding/gob"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/config"
	"github.com/assurrussa/goadmin/infrastructure/core/session/sessionpgsql"
	"github.com/assurrussa/goadmin/infrastructure/core/session/sessionredis"
	"github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/tests"
)

func TestPostgresSessionStorageLifecycle(t *testing.T) {
	ctx := t.Context()
	pool, _, cleanup := tests.PrepareDB(ctx, t, "BrowserSessions")
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		cleanup(cleanupCtx)
	})
	engine, ok := pool.DB().(outbox.StoragePgsqlDBPgxEnginePool)
	require.True(t, ok)
	db := stdlib.OpenDBFromPool(engine.Pool())
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	store, err := sessionpgsql.New(db)
	require.NoError(t, err)
	var encoded bytes.Buffer
	require.NoError(t, gob.NewEncoder(&encoded).Encode(map[any]any{"actor": "admin"}))
	payload := encoded.Bytes()
	require.NoError(t, store.SetWithContext(ctx, "active", payload, time.Hour))
	reopened, err := sessionpgsql.New(db)
	require.NoError(t, err)
	got, err := reopened.GetWithContext(ctx, "active")
	require.NoError(t, err)
	require.Equal(t, payload, got)

	require.NoError(t, store.SetWithContext(ctx, "expired", payload, time.Hour))
	_, err = db.ExecContext(ctx, `UPDATE goadmin_browser_sessions
SET expires_at = statement_timestamp() - interval '1 second' WHERE id = 'expired'`)
	require.NoError(t, err)
	got, err = store.GetWithContext(ctx, "expired")
	require.NoError(t, err)
	require.Nil(t, got)
	require.NoError(t, store.Cleanup(ctx))
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM goadmin_browser_sessions WHERE id = 'expired'`).Scan(&count))
	require.Zero(t, count)

	require.NoError(t, store.SetWithContext(ctx, "obsolete", []byte("invalid gob"), 0))
	got, err = store.GetWithContext(ctx, "obsolete")
	require.NoError(t, err)
	require.Nil(t, got)
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM goadmin_browser_sessions WHERE id = 'obsolete'`).Scan(&count))
	require.Zero(t, count)

	var workers sync.WaitGroup
	errors := make(chan error, 8)
	for range 8 {
		workers.Go(func() { errors <- store.SetWithContext(ctx, "shared", payload, time.Hour) })
	}
	workers.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	require.NoError(t, store.DeleteWithContext(ctx, "shared"))
	require.NoError(t, store.ResetWithContext(ctx))
	got, err = store.GetWithContext(ctx, "active")
	require.NoError(t, err)
	require.Nil(t, got)
	require.NoError(t, store.Close())
	require.NoError(t, db.PingContext(ctx))

	// A fresh runtime reads the prior runtime's saved browser session and
	// restores remember-me after Fiber writes its session-only cookie.
	cfg := config.Config{SessionTTL: time.Hour, SessionInactiveTTL: time.Minute}
	first := fiber.New()
	middleware, _ := sessionredis.CreateAdminSessionStoreWithStorage(store, cfg)
	first.Use(middleware)
	first.Get("/", func(c fiber.Ctx) error {
		session.FromContext(c).Set("actor", "admin")
		return c.SendStatus(http.StatusOK)
	})
	response, err := first.Test(httptest.NewRequestWithContext(ctx, http.MethodGet, "http://example.test/", nil))
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	var cookie *http.Cookie
	for _, candidate := range response.Cookies() {
		if candidate.Name == "session_id" {
			cookie = candidate
		}
	}
	require.NotNil(t, cookie)
	second := fiber.New()
	middleware, _ = sessionredis.CreateAdminSessionStoreWithStorage(reopened, cfg)
	second.Use(middleware)
	second.Get("/", func(c fiber.Ctx) error {
		require.Equal(t, "admin", session.FromContext(c).Get("actor"))
		return c.SendStatus(http.StatusOK)
	})
	request := httptest.NewRequestWithContext(ctx, http.MethodGet, "http://example.test/", nil)
	request.AddCookie(cookie)
	request.AddCookie(&http.Cookie{
		Name: "session_id_persistent", Value: "1", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	response, err = second.Test(request)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusOK, response.StatusCode)
	var persistentCookie *http.Cookie
	for _, candidate := range response.Cookies() {
		if candidate.Name == "session_id" {
			persistentCookie = candidate
		}
	}
	require.NotNil(t, persistentCookie)
	require.Equal(t, 3600, persistentCookie.MaxAge)
}
