package redis

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	redisv9 "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

const (
	firstShardAddress  = "one:6379"
	secondShardAddress = "two:6379"
)

func validConfig() Config {
	return Config{
		Addrs: []string{firstShardAddress, secondShardAddress}, ClientName: "admin", Database: 4,
		Username: "user", Password: "secret", PoolSize: 3, ConnTimeout: time.Second,
		ReadTimeout: 2 * time.Second, WriteTimeout: 3 * time.Second,
	}
}

func TestRingPreservesShardIdentityAndConfig(t *testing.T) {
	cfg := validConfig()
	ring, err := newPoolShards(t.Context(), cfg, func(options *redisv9.Options) *redisv9.Client {
		require.Equal(t, cfg.Database, options.DB)
		require.Equal(t, cfg.ClientName, options.ClientName)
		require.Equal(t, cfg.Username, options.Username)
		require.Equal(t, cfg.Password, options.Password)
		require.Equal(t, cfg.PoolSize, options.PoolSize)
		require.Equal(t, cfg.ConnTimeout, options.DialTimeout)
		require.Equal(t, cfg.ReadTimeout, options.ReadTimeout)
		require.Equal(t, cfg.WriteTimeout, options.WriteTimeout)
		return redisv9.NewClient(options)
	})
	require.NoError(t, err)
	defer func() { require.NoError(t, ring.Close()) }()
	require.Equal(t, map[string]string{
		firstShardAddress: firstShardAddress, secondShardAddress: secondShardAddress,
	}, ring.Options().Addrs)
	reversed := cfg
	reversed.Addrs = []string{secondShardAddress, firstShardAddress}
	require.Equal(t, ring.Options().Addrs, ringOptions(reversed, redisv9.NewClient).Addrs)
}

func TestRingRejectsInvalidConfig(t *testing.T) {
	for _, change := range []func(*Config){
		func(c *Config) { c.Addrs = nil }, func(c *Config) { c.Database = -1 },
		func(c *Config) { c.PoolSize = 0 }, func(c *Config) { c.PoolSize = 101 },
		func(c *Config) { c.ConnTimeout = time.Millisecond }, func(c *Config) { c.ReadTimeout = 2 * time.Minute },
		func(c *Config) { c.WriteTimeout = 0 },
	} {
		cfg := validConfig()
		change(&cfg)
		client, err := NewPoolShards(t.Context(), cfg)
		require.Error(t, err)
		require.Nil(t, client)
	}
}

type pingHook struct {
	calls   *atomic.Int32
	failure error
}

func (h pingHook) DialHook(next redisv9.DialHook) redisv9.DialHook { return next }
func (h pingHook) ProcessHook(next redisv9.ProcessHook) redisv9.ProcessHook {
	return func(ctx context.Context, cmd redisv9.Cmder) error {
		if cmd.Name() == "ping" {
			h.calls.Add(1)
			return h.failure
		}
		return next(ctx, cmd)
	}
}

func (h pingHook) ProcessPipelineHook(next redisv9.ProcessPipelineHook) redisv9.ProcessPipelineHook {
	return next
}

func TestInitialCheckVisitsAllShardsAndClosesOnFailure(t *testing.T) {
	cfg := validConfig()
	cfg.Check = true
	var calls atomic.Int32
	var clients []*redisv9.Client
	failure := errors.New("initial ping failed")
	ring, err := newPoolShards(t.Context(), cfg, func(options *redisv9.Options) *redisv9.Client {
		options.MaxRetries = -1
		options.Dialer = func(context.Context, string, string) (net.Conn, error) { return nil, errors.New("unexpected dial") }
		client := redisv9.NewClient(options)
		client.AddHook(pingHook{calls: &calls, failure: failure})
		clients = append(clients, client)
		return client
	})
	require.Nil(t, ring)
	require.ErrorIs(t, err, failure)
	require.Len(t, clients, 2)
	require.EqualValues(t, 2, calls.Load())
	for _, client := range clients {
		require.ErrorContains(t, client.Get(t.Context(), "probe").Err(), "client is closed")
	}
}
