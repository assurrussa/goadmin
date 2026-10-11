package connectionconfig

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
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

	conn, err := parsePgsqlDSN(dsn)
	if err != nil {
		return cfg, err
	}
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

// PgsqlRuntimeParams returns the supported startup settings without rebuilding
// their values from the scalar storage config. An absent DSN adds no overrides.
func PgsqlRuntimeParams(dsn string) (map[string]string, error) {
	if strings.TrimSpace(dsn) == "" {
		return map[string]string{}, nil
	}
	conn, err := parsePgsqlDSN(strings.TrimSpace(dsn))
	if err != nil {
		return nil, err
	}
	return conn.RuntimeParams, nil
}

func parsePgsqlDSN(dsn string) (*pgconn.Config, error) {
	conn, err := pgconn.ParseConfigWithOptions(dsn, pgconn.ParseConfigOptions{
		ConnStringAllowedKeys: []string{
			"host", "port", "database", "user", "password", "sslmode",
			"application_name", "timezone", "TimeZone",
			"statement_timeout", "lock_timeout", "idle_in_transaction_session_timeout",
			"search_path", "options",
		},
	})
	if err != nil {
		// Parser errors may contain DSN values, so never wrap or log them.
		return nil, errors.New("parse database dsn: invalid or unsupported PostgreSQL parameter")
	}
	// Startup messages use NUL-terminated strings; a decoded NUL could inject
	// another parameter and bypass validation of explicit DSN parameter names.
	if strings.IndexByte(conn.User, 0) >= 0 || strings.IndexByte(conn.Database, 0) >= 0 {
		return nil, errors.New("parse database dsn: NUL in PostgreSQL startup value")
	}
	for _, value := range conn.RuntimeParams {
		if strings.IndexByte(value, 0) >= 0 {
			return nil, errors.New("parse database dsn: NUL in PostgreSQL startup value")
		}
	}
	if value, ok := conn.RuntimeParams["TimeZone"]; ok {
		if other, exists := conn.RuntimeParams["timezone"]; exists && value != other {
			return nil, errors.New("conflicting database dsn parameters timezone and TimeZone")
		}
		conn.RuntimeParams["timezone"] = value
		delete(conn.RuntimeParams, "TimeZone")
	}
	for key, value := range conn.RuntimeParams {
		switch key {
		case "application_name", "timezone", "TimeZone",
			"statement_timeout", "lock_timeout", "idle_in_transaction_session_timeout":
		case "search_path":
			if value != "public" {
				return nil, errors.New("unsupported database dsn parameter search_path: only public is supported")
			}
		default:
			// Only a parameter name is exposed, never its value or the DSN.
			return nil, fmt.Errorf("unsupported database dsn parameter %q", key)
		}
	}
	return conn, nil
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
