//go:build integration

package browserstate

import (
	"bytes"
	"context"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	redis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func integrationKeys(t *testing.T) goauth.KeyRing {
	t.Helper()
	keys, err := goauth.NewKeyRing("test", goauth.Key{ID: "test", Material: bytes.Repeat([]byte{7}, 32)})
	require.NoError(t, err)
	return keys
}

func exerciseAtomicState(t *testing.T, s Store) string {
	t.Helper()
	id := uuid.NewString()
	ctx := t.Context()
	record := Record{SubjectID: goauth.NewSubjectID(), Tokens: goauth.TokenPair{AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh"}}
	require.NoError(t, s.Create(ctx, id, record, time.Hour))
	t.Cleanup(func() { _ = s.Delete(context.Background(), id) })
	start := make(chan struct{})
	var winners atomic.Int32
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			<-start
			ok, err := s.Claim(ctx, id, 1, "winner")
			require.NoError(t, err)
			if ok {
				winners.Add(1)
			}
		})
	}
	close(start)
	wg.Wait()
	require.Equal(t, int32(1), winners.Load())
	current, ok, err := s.Load(ctx, id)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "winner", current.Owner)
	ok, err = s.Complete(ctx, id, 1, "loser", record)
	require.NoError(t, err)
	require.False(t, ok)
	record.Tokens.RefreshToken = "synthetic-rotated-refresh"
	ok, err = s.Complete(ctx, id, 1, "winner", record)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = s.Complete(ctx, id, 1, "winner", record)
	require.NoError(t, err)
	require.False(t, ok, "stale owner overwrote newer state")
	current, ok, err = s.Load(ctx, id)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, int64(2), current.Version)
	require.Equal(t, record.Tokens.RefreshToken, current.Tokens.RefreshToken)
	ok, err = s.Claim(ctx, id, 1, "old-version")
	require.NoError(t, err)
	require.False(t, ok)
	return id
}

func TestRedisStateUsesAtomicOwnerVersionAndRetainsTTL(t *testing.T) {
	address := os.Getenv("GOAUTH_TEST_REDIS_ADDRESS")
	require.NotEmpty(t, address, "integration Redis address is required")
	conn := redis.NewClient(&redis.Options{Addr: address})
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.Ping(t.Context()).Err())
	id := exerciseAtomicState(t, NewRedis(conn, integrationKeys(t)))
	raw, err := conn.Get(t.Context(), redisKey(id)).Result()
	require.NoError(t, err)
	require.NotContains(t, raw, "synthetic-access")
	require.NotContains(t, raw, "synthetic-refresh")
	ttl, err := conn.PTTL(t.Context(), redisKey(id)).Result()
	require.NoError(t, err)
	require.Positive(t, ttl)
	require.LessOrEqual(t, ttl, time.Hour)
}

func TestPostgresStateUsesIndependentConnectionsAndEncryptedRecords(t *testing.T) {
	dsn := os.Getenv("GOAUTH_TEST_POSTGRES_DSN")
	require.NotEmpty(t, dsn, "integration DSN is required")
	admin, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = admin.Close() })
	schema := "admin_state_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = admin.ExecContext(t.Context(), "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = admin.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("pgx", u.String())
	require.NoError(t, err)
	db.SetMaxOpenConns(16)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.ExecContext(t.Context(), `CREATE TABLE goadmin_browser_auth_state(id text PRIMARY KEY,version bigint NOT NULL,owner text NOT NULL,record jsonb NOT NULL,expires_at timestamptz NOT NULL)`)
	require.NoError(t, err)
	id := exerciseAtomicState(t, NewPostgres(db, integrationKeys(t)))
	var raw string
	err = db.QueryRowContext(t.Context(), `SELECT record::text FROM goadmin_browser_auth_state WHERE id=$1`, id).Scan(&raw)
	require.NoError(t, err)
	require.NotContains(t, raw, "synthetic-access")
	require.NotContains(t, raw, "synthetic-refresh")
}
