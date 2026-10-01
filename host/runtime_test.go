package host_test

import (
	"context"
	"strings"
	"testing"
	"time"

	redisv9 "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	adminhost "github.com/assurrussa/goadmin/host"
)

func TestRuntimeConstructorsCompile(t *testing.T) {
	t.Parallel()

	var _ adminhost.PgsqlConfig
	var _ adminhost.RedisConfig
	var _ adminhost.CSRFConfig
	var _ adminhost.OutboxConfig
	var _ adminhost.OutboxRuntime
	var _ *adminhost.InMemoryEventStream
	var _ adminhost.LocalUploadsConfig
	var _ adminhost.LocalUploadsRuntime
	_ = adminhost.NewPgsqlClient
	_ = adminhost.NewTxManager
	_ = adminhost.NewRedisClient
	_ = adminhost.NewCSRFService
	_ = adminhost.NewEventStream
	_ = adminhost.NewNotificationManager
	_ = adminhost.NewOutbox
	_ = adminhost.NewOutboxRuntime
	_ = adminhost.NewLocalUploads
	_ = adminhost.DiscardLogger
}

func TestNewOutboxRuntimeRejectsMissingCoreDependencies(t *testing.T) {
	t.Parallel()

	runtime, err := adminhost.NewOutboxRuntime(nil, nil, nil, adminhost.OutboxConfig{})
	require.Nil(t, runtime)
	require.ErrorContains(t, err, "tx manager is required")
}

func TestNewTxManagerHandlesNilDatabase(t *testing.T) {
	t.Parallel()

	require.Nil(t, adminhost.NewTxManager(nil))
}

func TestNewOutboxRejectsMissingCoreDependencies(t *testing.T) {
	t.Parallel()

	outbox, err := adminhost.NewOutbox(nil, nil, nil, adminhost.OutboxConfig{})

	require.Nil(t, outbox)
	require.Error(t, err)
	require.Contains(t, err.Error(), "tx manager is required")
}

func TestNewCSRFServiceUsesStableDefaults(t *testing.T) {
	t.Parallel()

	service, err := adminhost.NewCSRFService(adminhost.CSRFConfig{
		AppDomain:      "example.com",
		SecretKey:      strings.Repeat("s", 32),
		AllowedOrigins: []string{"https://example.com"}, //nolint:goconst // required
		TokenTTL:       time.Hour,
	}, nil)

	require.NoError(t, err)
	require.NotNil(t, service)

	req := adminhost.CSRFRequest{SessionID: "opaque-session-digest", UserID: adminhost.AdminUUID{1}}
	token, err := service.Create(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, "example.com", token.Domain)
	claims, valid, err := service.Check(context.Background(), req, token.Token)
	require.NoError(t, err)
	require.True(t, valid)
	require.Equal(t, req.UserID, claims.UserID)
	require.WithinDuration(t, time.Now().Add(time.Hour), claims.ExpiresAt.Time, 2*time.Second)
}

func TestNewRedisClientValidatesAddresses(t *testing.T) {
	t.Parallel()

	client, err := adminhost.NewRedisClient(context.Background(), adminhost.RedisConfig{})

	require.Nil(t, client)
	require.Error(t, err)
}

func TestNewPgsqlClientReportsInvalidDSN(t *testing.T) {
	t.Parallel()

	client, err := adminhost.NewPgsqlClient(context.Background(), adminhost.PgsqlConfig{
		DSN: "postgres://%zz",
	}, nil)

	require.Nil(t, client)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse database dsn")
}

func TestNewRedisClientReportsInvalidDSN(t *testing.T) {
	t.Parallel()

	client, err := adminhost.NewRedisClient(context.Background(), adminhost.RedisConfig{
		DSN: "rediss://redis.local:6379/0",
	})

	require.Nil(t, client)
	require.Error(t, err)
	require.Contains(t, err.Error(), "rediss://")
}

func TestNewRedisClientReportsInvalidDSNOption(t *testing.T) {
	t.Parallel()

	client, err := adminhost.NewRedisClient(context.Background(), adminhost.RedisConfig{
		DSN: "redis://redis.local:6379/0?pool_size=bad",
	})

	require.Nil(t, client)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse redis dsn options")
}

func TestNewRedisClientUsesStableDefaults(t *testing.T) {
	const redisAddress = "redis.local:6379"

	t.Parallel()
	client, err := adminhost.NewRedisClient(t.Context(), adminhost.RedisConfig{
		Addrs: []string{redisAddress}, DisableCheck: true, Database: 2,
	})
	require.NoError(t, err)
	defer func() { require.NoError(t, client.Close()) }()
	ring, ok := client.(*redisv9.Ring)
	require.True(t, ok)
	options := ring.Options()
	require.Equal(t, map[string]string{redisAddress: redisAddress}, options.Addrs)
	require.Equal(t, "goadmin-host-redis", options.ClientName)
	require.Equal(t, 5*time.Second, options.DialTimeout)
	require.Equal(t, 3*time.Second, options.ReadTimeout)
	require.Equal(t, 3*time.Second, options.WriteTimeout)
	require.Equal(t, 10, options.PoolSize)
	require.NoError(t, ring.ForEachShard(t.Context(), func(_ context.Context, shard *redisv9.Client) error {
		require.Equal(t, 2, shard.Options().DB)
		return nil
	}))
}
