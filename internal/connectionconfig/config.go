package connectionconfig

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/assurrussa/goadmin/infrastructure/outbox"
	appredis "github.com/assurrussa/goadmin/infrastructure/redis"
)

// PgsqlConfig describes PostgreSQL settings accepted by reusable goadmin surfaces.
type PgsqlConfig struct {
	DSN                 string
	Address             string
	Username            string
	Password            string
	Database            string
	SSLMode             string
	DebugMode           bool
	MinConnectionsCount int32
	MaxConnectionsCount int32
	TLSCert             string
	TLSKey              string
	MaxConnIdleTime     time.Duration
	MaxConnLifeTime     time.Duration
}

// RedisConfig describes Redis settings accepted by reusable goadmin surfaces.
type RedisConfig struct {
	DSN          string
	Addrs        []string
	ClientName   string
	ConnTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	Database     int
	PoolSize     int
	Username     string
	Password     string
	DisableCheck bool
}

// PgsqlStorageConfig converts public PostgreSQL config to the local outbox storage config.
func PgsqlStorageConfig(cfg PgsqlConfig) (outbox.StoragePgsqlConfig, error) {
	cfg, err := NormalizePgsql(cfg)
	if err != nil {
		return outbox.StoragePgsqlConfig{}, err
	}

	return outbox.StoragePgsqlConfig{
		Address:             cfg.Address,
		Username:            cfg.Username,
		Password:            cfg.Password,
		Database:            cfg.Database,
		SSLMode:             cfg.SSLMode,
		DebugMode:           cfg.DebugMode,
		MinConnectionsCount: cfg.MinConnectionsCount,
		MaxConnectionsCount: cfg.MaxConnectionsCount,
		TLSCert:             cfg.TLSCert,
		TLSKey:              cfg.TLSKey,
		MaxConnIdleTime:     cfg.MaxConnIdleTime,
		MaxConnLifeTime:     cfg.MaxConnLifeTime,
	}, nil
}

// NormalizePgsql applies DSN values to PostgreSQL config when DSN is present.
func NormalizePgsql(cfg PgsqlConfig) (PgsqlConfig, error) {
	dsn := strings.TrimSpace(cfg.DSN)
	if dsn == "" {
		return cfg, nil
	}

	parsed, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return cfg, fmt.Errorf("parse database dsn: %w", err)
	}

	conn := parsed.ConnConfig
	if conn.Host != "" {
		cfg.Address = conn.Host
		if conn.Port > 0 && !strings.Contains(conn.Host, "/") {
			cfg.Address = net.JoinHostPort(conn.Host, strconv.Itoa(int(conn.Port)))
		}
	}
	if conn.User != "" {
		cfg.Username = conn.User
	}
	if conn.Password != "" {
		cfg.Password = conn.Password
	}
	if conn.Database != "" {
		cfg.Database = conn.Database
	}
	if sslMode := dsnSSLMode(dsn); sslMode != "" {
		cfg.SSLMode = sslMode
	}

	return cfg, nil
}

// RedisPoolConfig converts public Redis config to the local redis pool config.
func RedisPoolConfig(cfg RedisConfig) (appredis.Config, error) {
	cfg, err := NormalizeRedis(cfg)
	if err != nil {
		return appredis.Config{}, err
	}

	return appredis.Config{
		Addrs:        cfg.Addrs,
		ClientName:   cfg.ClientName,
		ConnTimeout:  cfg.ConnTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		Database:     cfg.Database,
		PoolSize:     cfg.PoolSize,
		Username:     cfg.Username,
		Password:     cfg.Password,
		Check:        !cfg.DisableCheck,
	}, nil
}

// NormalizeRedis applies DSN values to Redis config when DSN is present.
func NormalizeRedis(cfg RedisConfig) (RedisConfig, error) {
	dsn := strings.TrimSpace(cfg.DSN)
	if dsn == "" {
		return cfg, nil
	}

	parsed, err := url.Parse(dsn)
	if err != nil {
		return cfg, fmt.Errorf("parse redis dsn: %w", err)
	}

	switch parsed.Scheme {
	case "redis":
	case "rediss":
		return cfg, errors.New("redis dsn rediss:// requires TLS config that goadmin host RedisConfig does not expose")
	default:
		return cfg, fmt.Errorf("unsupported redis dsn scheme %q", parsed.Scheme)
	}

	options, err := goredis.ParseURL(dsn)
	if err != nil {
		return cfg, fmt.Errorf("parse redis dsn options: %w", err)
	}
	cfg.Addrs = []string{options.Addr}

	if options.Username != "" {
		cfg.Username = options.Username
	}
	if options.Password != "" {
		cfg.Password = options.Password
	}
	if redisDSNHasDatabase(parsed) {
		cfg.Database = options.DB
	}
	if options.ClientName != "" {
		cfg.ClientName = options.ClientName
	}
	if options.DialTimeout != 0 {
		cfg.ConnTimeout = options.DialTimeout
	}
	if options.ReadTimeout != 0 {
		cfg.ReadTimeout = options.ReadTimeout
	}
	if options.WriteTimeout != 0 {
		cfg.WriteTimeout = options.WriteTimeout
	}
	if options.PoolSize > 0 {
		cfg.PoolSize = options.PoolSize
	}

	return cfg, nil
}

func redisDSNHasDatabase(parsed *url.URL) bool {
	if parsed == nil {
		return false
	}

	return strings.Trim(strings.TrimSpace(parsed.Path), "/") != "" || strings.TrimSpace(parsed.Query().Get("db")) != ""
}

func dsnSSLMode(dsn string) string {
	parsed, err := url.Parse(dsn)
	if err == nil {
		if sslMode := strings.TrimSpace(parsed.Query().Get("sslmode")); sslMode != "" {
			return sslMode
		}
	}

	for _, field := range strings.Fields(dsn) {
		key, value, ok := strings.Cut(field, "=")
		if ok && strings.EqualFold(strings.TrimSpace(key), "sslmode") {
			return strings.Trim(strings.TrimSpace(value), "'\"")
		}
	}

	return ""
}
