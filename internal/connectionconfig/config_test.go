package connectionconfig_test

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/internal/connectionconfig"
)

func TestPgsqlStorageConfigFromDSN(t *testing.T) {
	t.Parallel()

	password := strings.Repeat("s", 6)
	cfg, err := connectionconfig.PgsqlStorageConfig(connectionconfig.PgsqlConfig{
		DSN:                 postgresDSN("goadmin", password, "pgsql.local:15432", "goadmindb", "require"),
		MinConnectionsCount: 2,
		MaxConnectionsCount: 7,
		MaxConnIdleTime:     2 * time.Minute,
		MaxConnLifeTime:     30 * time.Minute,
	})
	require.NoError(t, err)

	assert.Equal(t, "pgsql.local:15432", cfg.Address)
	assert.Equal(t, "goadmin", cfg.Username)
	assert.Equal(t, password, cfg.Password)
	assert.Equal(t, "goadmindb", cfg.Database)
	assert.Equal(t, "require", cfg.SSLMode)
	assert.Equal(t, int32(2), cfg.MinConnectionsCount)
	assert.Equal(t, int32(7), cfg.MaxConnectionsCount)
	assert.Equal(t, 2*time.Minute, cfg.MaxConnIdleTime)
	assert.Equal(t, 30*time.Minute, cfg.MaxConnLifeTime)
}

func TestPgsqlStorageConfigReportsInvalidDSN(t *testing.T) {
	t.Parallel()

	_, err := connectionconfig.PgsqlStorageConfig(connectionconfig.PgsqlConfig{DSN: "postgres://%zz"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse database dsn")
}

func TestRedisConfigFromDSN(t *testing.T) {
	t.Parallel()

	password := strings.Repeat("s", 6)
	cfg, err := connectionconfig.NormalizeRedis(connectionconfig.RedisConfig{
		DSN: redisDSN("default", password, "redis.local:6380", "3", map[string]string{
			"client_name":   "admin-cache",
			"dial_timeout":  "6s",
			"read_timeout":  "4s",
			"write_timeout": "5s",
			"pool_size":     "16",
		}),
		DisableCheck: true,
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"redis.local:6380"}, cfg.Addrs)
	assert.Equal(t, "admin-cache", cfg.ClientName)
	assert.Equal(t, 6*time.Second, cfg.ConnTimeout)
	assert.Equal(t, 4*time.Second, cfg.ReadTimeout)
	assert.Equal(t, 5*time.Second, cfg.WriteTimeout)
	assert.Equal(t, "default", cfg.Username)
	assert.Equal(t, password, cfg.Password)
	assert.Equal(t, 3, cfg.Database)
	assert.Equal(t, 16, cfg.PoolSize)
	assert.True(t, cfg.DisableCheck)
}

func TestRedisConfigFromDSNDatabaseQuery(t *testing.T) {
	t.Parallel()

	cfg, err := connectionconfig.NormalizeRedis(connectionconfig.RedisConfig{
		DSN: redisDSN("", "", "redis.local:6379", "", map[string]string{
			"db": "2",
		}),
	})
	require.NoError(t, err)

	assert.Equal(t, 2, cfg.Database)
}

func TestRedisConfigRejectsUnsupportedDSN(t *testing.T) {
	t.Parallel()

	_, err := connectionconfig.NormalizeRedis(connectionconfig.RedisConfig{DSN: "rediss://redis.local:6379/0"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "rediss://")
}

func TestRedisConfigRejectsInvalidDSNOption(t *testing.T) {
	t.Parallel()

	_, err := connectionconfig.NormalizeRedis(connectionconfig.RedisConfig{
		DSN: redisDSN("", "", "redis.local:6379", "", map[string]string{
			"pool_size": "bad",
		}),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse redis dsn options")
}

func postgresDSN(username string, password string, host string, database string, sslMode string) string {
	parsed := url.URL{
		Scheme: "postgresql",
		User:   url.UserPassword(username, password),
		Host:   host,
		Path:   database,
	}
	query := parsed.Query()
	query.Set("sslmode", sslMode)
	parsed.RawQuery = query.Encode()

	return parsed.String()
}

func redisDSN(username string, password string, host string, database string, query map[string]string) string {
	parsed := &url.URL{
		Scheme: "redis",
		Host:   host,
		Path:   database,
	}
	if username != "" || password != "" {
		parsed.User = url.UserPassword(username, password)
	}
	if len(query) > 0 {
		values := parsed.Query()
		for key, value := range query {
			values.Set(key, value)
		}
		parsed.RawQuery = values.Encode()
	}

	return parsed.String()
}
