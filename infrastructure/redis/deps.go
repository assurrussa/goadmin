package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	redisv9 "github.com/redis/go-redis/v9"
)

// Config contains the settings used by the admin session Ring.
type Config struct {
	Addrs        []string
	ClientName   string
	ConnTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	Database     int
	PoolSize     int
	Username     string
	Password     string
	Check        bool
}

// ClientContract is the direct go-redis client contract supplied by hosts.
type ClientContract = redisv9.UniversalClient

type ClientShardContract interface {
	redisv9.UniversalClient
	ForEachShard(ctx context.Context, fn func(ctx context.Context, client *redisv9.Client) error) error
}

// NewPoolShards preserves address-based shard identities and a database per shard.
// The caller owns the returned client; a failed initial check closes it here.
func NewPoolShards(ctx context.Context, cfg Config) (*redisv9.Ring, error) {
	return newPoolShards(ctx, cfg, redisv9.NewClient)
}

func newPoolShards(ctx context.Context, cfg Config, newClient func(*redisv9.Options) *redisv9.Client) (*redisv9.Ring, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("redis validate options: %w", err)
	}
	var shards []*redisv9.Client
	client := redisv9.NewRing(ringOptions(cfg, func(options *redisv9.Options) *redisv9.Client {
		shard := newClient(options)
		shards = append(shards, shard)
		return shard
	}))
	if cfg.Check {
		var failures []error
		// Check the complete configured topology, even if Ring heartbeat marks a shard down.
		for _, shard := range shards {
			if err := shard.Ping(ctx).Err(); err != nil {
				failures = append(failures, err)
			}
		}
		if err := errors.Join(failures...); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("failed to connect to redis: %w", err)
		}
	}
	return client, nil
}

func ringOptions(cfg Config, newClient func(*redisv9.Options) *redisv9.Client) *redisv9.RingOptions {
	addrs := make(map[string]string, len(cfg.Addrs))
	for _, address := range cfg.Addrs {
		addrs[address] = address
	}
	return &redisv9.RingOptions{
		Addrs: addrs, ClientName: cfg.ClientName, Username: cfg.Username, Password: cfg.Password,
		PoolSize: cfg.PoolSize, DialTimeout: cfg.ConnTimeout, ReadTimeout: cfg.ReadTimeout, WriteTimeout: cfg.WriteTimeout,
		NewClient: func(options *redisv9.Options) *redisv9.Client {
			options.DB = cfg.Database
			return newClient(options)
		},
		OnConnect: func(ctx context.Context, conn *redisv9.Conn) error { return conn.Ping(ctx).Err() },
	}
}

func validateConfig(cfg Config) error {
	if len(cfg.Addrs) == 0 {
		return errors.New("address must contain at least one Redis address")
	}
	if cfg.Database < 0 {
		return errors.New("database must be non-negative")
	}
	if cfg.PoolSize < 1 || cfg.PoolSize > 100 {
		return errors.New("poolSize must be between 1 and 100")
	}
	for _, timeout := range []struct {
		name  string
		value time.Duration
	}{
		{"connTimeout", cfg.ConnTimeout}, {"readTimeout", cfg.ReadTimeout}, {"writeTimeout", cfg.WriteTimeout},
	} {
		if timeout.value < time.Second || timeout.value > time.Minute {
			return fmt.Errorf("%s must be between 1s and 1m", timeout.name)
		}
	}
	return nil
}
