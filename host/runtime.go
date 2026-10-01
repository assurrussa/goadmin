package host

import (
	"context"
	"errors"
	"fmt"
	"time"

	logger "github.com/assurrussa/gologger"
	"github.com/assurrussa/gonotify/transport/notifyhub"
	inmemeventstream "github.com/assurrussa/gowebsocket/eventstream/inmem"

	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	redis "github.com/assurrussa/goadmin/infrastructure/redis"
	"github.com/assurrussa/goadmin/internal/connectionconfig"
	csrfservice "github.com/assurrussa/goadmin/internal/csrf"
)

// PgsqlConfig contains the minimal PostgreSQL settings needed by a clean host.
type PgsqlConfig struct {
	DSN      string
	Address  string
	Username string
	Password string
	Database string
	SSLMode  string
	Env      string
}

// RedisConfig contains the minimal Redis settings needed by a clean host.
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

// CSRFConfig contains the stable CSRF settings needed by the embedded admin.
type CSRFConfig struct {
	AppDomain      string
	SecretKey      string
	AllowedOrigins []string
	TokenTTL       time.Duration
}

// OutboxConfig contains queue worker settings for built-in admin jobs.
type OutboxConfig struct {
	Workers    int
	IdleTime   time.Duration
	ReserveFor time.Duration
}

// InMemoryEventStream is the default in-process event stream implementation for starter hosts.
type InMemoryEventStream = inmemeventstream.Service

// DiscardLogger returns a logger that drops all output.
func DiscardLogger() Logger {
	return logger.Discard()
}

// NewPgsqlClient creates the PostgreSQL client used by the admin runtime.
func NewPgsqlClient(ctx context.Context, cfg PgsqlConfig, lg Logger) (StoragePgsqlClient, error) {
	if lg == nil {
		lg = DiscardLogger()
	}
	sslMode := cfg.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	env := cfg.Env
	if env == "" {
		env = "development"
	}

	storageCfg, err := connectionconfig.PgsqlStorageConfig(connectionconfig.PgsqlConfig{
		DSN:      cfg.DSN,
		Address:  cfg.Address,
		Username: cfg.Username,
		Password: cfg.Password,
		Database: cfg.Database,
		SSLMode:  sslMode,
	})
	if err != nil {
		return nil, err
	}
	if storageCfg.SSLMode == "" {
		storageCfg.SSLMode = sslMode
	}
	if storageCfg.MinConnectionsCount <= 0 {
		storageCfg.MinConnectionsCount = 5
	}
	if storageCfg.MaxConnectionsCount <= 0 {
		storageCfg.MaxConnectionsCount = 10
	}
	if storageCfg.MaxConnIdleTime <= 0 {
		storageCfg.MaxConnIdleTime = 5 * time.Minute
	}
	if storageCfg.MaxConnLifeTime <= 0 {
		storageCfg.MaxConnLifeTime = time.Hour
	}

	client, err := outbox.PgsqlCreateWithConfig(
		ctx,
		storageCfg,
		outbox.WithPgsqlEnvironment(env),
		outbox.WithPgsqlLogger(outbox.WrapNamed(lg)),
	)
	if err != nil {
		return nil, fmt.Errorf("create pgsql client: %w", err)
	}

	return client, nil
}

// NewTxManager creates a transaction manager for the PostgreSQL client.
func NewTxManager(db StoragePgsqlClient) StoragePgsqlTxManager {
	if db == nil {
		return nil
	}

	return outbox.PgsqlTrxNew(db.DB())
}

// NewRedisClient creates the Redis client used by admin sessions.
func NewRedisClient(ctx context.Context, cfg RedisConfig) (RedisClient, error) {
	redisCfg, err := connectionconfig.RedisPoolConfig(connectionconfig.RedisConfig{
		DSN:          cfg.DSN,
		Addrs:        cfg.Addrs,
		ClientName:   cfg.ClientName,
		ConnTimeout:  cfg.ConnTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		Database:     cfg.Database,
		PoolSize:     cfg.PoolSize,
		Username:     cfg.Username,
		Password:     cfg.Password,
		DisableCheck: cfg.DisableCheck,
	})
	if err != nil {
		return nil, err
	}
	applyRedisDefaults(&redisCfg)

	client, err := redis.NewPoolShards(ctx, redisCfg)
	if err != nil {
		return nil, fmt.Errorf("create redis client: %w", err)
	}

	return client, nil
}

func applyRedisDefaults(cfg *redis.Config) {
	if cfg.ClientName == "" {
		cfg.ClientName = "goadmin-host-redis"
	}
	if cfg.ConnTimeout <= 0 {
		cfg.ConnTimeout = 5 * time.Second
	}
	if cfg.ReadTimeout <= 0 {
		cfg.ReadTimeout = 3 * time.Second
	}
	if cfg.WriteTimeout <= 0 {
		cfg.WriteTimeout = 3 * time.Second
	}
	if cfg.PoolSize <= 0 {
		cfg.PoolSize = 10
	}
}

// NewCSRFService creates the CSRF service used by the embedded admin runtime.
func NewCSRFService(cfg CSRFConfig, lg Logger) (*CSRFService, error) {
	if lg == nil {
		lg = DiscardLogger()
	}
	tokenTTL := cfg.TokenTTL
	if tokenTTL <= 0 {
		tokenTTL = 24 * time.Hour
	}

	service, err := csrfservice.New(csrfservice.NewOptions(
		cfg.AppDomain,
		cfg.SecretKey,
		cfg.AllowedOrigins,
		lg,
		csrfservice.WithTokenTTL(tokenTTL),
	))
	if err != nil {
		return nil, fmt.Errorf("create csrf service: %w", err)
	}

	return service, nil
}

// NewEventStream creates the default in-memory event stream.
func NewEventStream() *InMemoryEventStream {
	return inmemeventstream.New()
}

// NewNotificationManager validates and creates a NotifyHub transport without sending requests.
// The host owns any HTTP client supplied through cfg.
func NewNotificationManager(cfg NotificationConfig) (NotificationManager, error) {
	client, err := notifyhub.New(cfg)
	if err != nil {
		return nil, err
	}
	return client, nil
}

// NewOutbox creates the built-in admin outbox service.
func NewOutbox(tx StoragePgsqlTxManager, db StoragePgsqlClient, lg Logger, cfg OutboxConfig) (Outbox, error) {
	return NewOutboxRuntime(tx, db, lg, cfg)
}

// OutboxRuntime adds worker lifecycle control to the enqueue and registration facade.
// The host must run the worker and include Readiness in its process health check.
type OutboxRuntime interface {
	Outbox
	Run(ctx context.Context) error
	Readiness(ctx context.Context) error
	BeginDrain()
}

// NewOutboxRuntime creates the built-in admin outbox with its worker lifecycle.
func NewOutboxRuntime(tx StoragePgsqlTxManager, db StoragePgsqlClient, lg Logger, cfg OutboxConfig) (OutboxRuntime, error) {
	if tx == nil {
		return nil, errors.New("create outbox service: tx manager is required")
	}
	if db == nil {
		return nil, errors.New("create outbox service: database client is required")
	}
	if lg == nil {
		lg = DiscardLogger()
	}
	jobsRepo := outbox.NewPgsqlJobsRepo(db)
	failedRepo := outbox.NewPgsqlJobsFailedRepo(db)

	opts := []outbox.OptOptionsSetter{
		outbox.WithTransactor(tx),
		outbox.WithJobsRepo(jobsRepo),
		outbox.WithJobsStatRepo(jobsRepo),
		outbox.WithJobsFailedRepo(failedRepo),
		outbox.WithLogger(outbox.WrapNamed(lg)),
	}
	if cfg.Workers > 0 {
		opts = append(opts, outbox.WithWorkers(cfg.Workers))
	}
	if cfg.IdleTime > 0 {
		opts = append(opts, outbox.WithIdleTime(cfg.IdleTime))
	}
	if cfg.ReserveFor > 0 {
		opts = append(opts, outbox.WithReserveFor(cfg.ReserveFor))
	}

	service, err := outbox.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("create outbox service: %w", err)
	}

	return service, nil
}
