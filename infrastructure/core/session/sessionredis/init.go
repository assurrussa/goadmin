package sessionredis

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/extractors"
	"github.com/gofiber/fiber/v3/middleware/session"
	redisv9 "github.com/redis/go-redis/v9"

	"github.com/assurrussa/goadmin/config"
	redis "github.com/assurrussa/goadmin/infrastructure/redis"
)

const (
	sessionKeyPrefix               = "goadmin:session:"
	sessionPersistenceCookieSuffix = "_persistent"
	resetScanCount                 = 100
)

func CreateAdminSessionStore(rdb redis.ClientContract, cfg config.Config) (fiber.Handler, *session.Store) {
	return CreateAdminSessionStoreWithStorage(&storage{conn: rdb}, cfg)
}

// CreateAdminSessionStoreWithStorage applies the common browser cookie policy.
// The caller owns the storage backend and its lifecycle.
func CreateAdminSessionStoreWithStorage(backend fiber.Storage, cfg config.Config) (fiber.Handler, *session.Store) {
	appDomain := cfg.Domain
	sessionName := cfg.SessionName
	if sessionName == "" {
		sessionName = "session_id"
	}

	if cfg.IsProduction() {
		appDomain = ""
		if !strings.HasPrefix(sessionName, "__Host-") {
			sessionName = "__Host-" + sessionName
		}
	}

	storage := &persistentCookieStorage{
		Storage:       backend,
		cookieName:    sessionName,
		cookieDomain:  appDomain,
		cookieSecure:  cfg.IsProduction(),
		persistentTTL: sessionCookieTTL(cfg),
	}
	return session.NewWithStore(session.Config{
		// Storage
		Storage: storage,
		// Session Management
		IdleTimeout:     cfg.SessionInactiveTTL, // Inactivity timeout
		AbsoluteTimeout: cfg.SessionTTL,         // Maximum session duration
		// Cookie Settings
		CookiePath:        "/",
		CookieDomain:      appDomain,
		CookieSessionOnly: true, // Remember-me is applied per browser session below.
		// Security
		CookieSecure:   cfg.IsProduction(), // HTTPS only (required in production)
		CookieHTTPOnly: true,               // No JavaScript access (prevents XSS)
		CookieSameSite: "Lax",              // CSRF protection
		// Session ID
		Extractor: extractors.FromCookie(sessionName),
		// KeyLookup: "cookie:" + sessionName,
	})
}

// Fiber sets a session cookie before it calls Storage.SetWithContext. Restore
// remember-me after successful writes, including writes from other session users.
type persistentCookieStorage struct {
	fiber.Storage
	cookieName    string
	cookieDomain  string
	cookieSecure  bool
	persistentTTL time.Duration
}

func (s *persistentCookieStorage) SetWithContext(ctx context.Context, key string, val []byte, exp time.Duration) error {
	if err := s.Storage.SetWithContext(ctx, key, val, exp); err != nil {
		return err
	}
	c, ok := ctx.(fiber.Ctx)
	if !ok || c.Cookies(s.cookieName+sessionPersistenceCookieSuffix) != "1" {
		return nil
	}
	ttl := s.persistentTTL
	c.Cookie(&fiber.Cookie{
		Name: s.cookieName, Value: key, Path: "/", Domain: s.cookieDomain,
		Secure: s.cookieSecure, HTTPOnly: true, SameSite: "Lax",
		MaxAge: int(ttl.Seconds()), Expires: time.Now().Add(ttl),
	})
	return nil
}

func sessionCookieTTL(cfg config.Config) time.Duration {
	if cfg.SessionTTL > 0 {
		return cfg.SessionTTL
	}
	if cfg.SessionInactiveTTL > 0 {
		return cfg.SessionInactiveTTL
	}
	return 24 * time.Hour
}

// Временно пока не будет норм замена.
type storage struct {
	conn redisv9.UniversalClient
}

func (s *storage) Get(key string) ([]byte, error) {
	return s.GetWithContext(context.Background(), key)
}

func (s *storage) GetWithContext(ctx context.Context, key string) ([]byte, error) {
	if len(key) == 0 {
		return nil, nil
	}

	val, err := s.conn.Get(ctx, sessionKeyPrefix+key).Bytes()
	if errors.Is(err, redisv9.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// Match Fiber's gob map decoder before handing bytes to its middleware.
	// Missing registered legacy types must expire this browser session rather
	// than fail every subsequent request carrying its cookie.
	var data map[any]any
	if decodeErr := gob.NewDecoder(bytes.NewReader(val)).Decode(&data); decodeErr != nil {
		if deleteErr := s.DeleteWithContext(ctx, key); deleteErr != nil {
			return nil, deleteErr
		}
		return nil, nil
	}
	return val, nil
}

func (s *storage) Set(key string, val []byte, exp time.Duration) error {
	return s.SetWithContext(context.Background(), key, val, exp)
}

func (s *storage) SetWithContext(ctx context.Context, key string, val []byte, exp time.Duration) error {
	if len(key) == 0 || len(val) == 0 {
		return nil
	}

	return s.conn.Set(ctx, sessionKeyPrefix+key, val, exp).Err()
}

func (s *storage) Delete(key string) error {
	return s.DeleteWithContext(context.Background(), key)
}

func (s *storage) DeleteWithContext(ctx context.Context, key string) error {
	if len(key) == 0 {
		return nil
	}

	return s.conn.Del(ctx, sessionKeyPrefix+key).Err()
}

func (s *storage) Reset() error {
	return s.ResetWithContext(context.Background())
}

func (s *storage) ResetWithContext(ctx context.Context) error {
	if shards, ok := s.conn.(redis.ClientShardContract); ok {
		return shards.ForEachShard(ctx, func(ctx context.Context, client *redisv9.Client) error {
			return resetSessions(ctx, client)
		})
	}
	if cluster, ok := s.conn.(interface {
		ForEachMaster(ctx context.Context, fn func(ctx context.Context, client *redisv9.Client) error) error
	}); ok {
		return cluster.ForEachMaster(ctx, func(ctx context.Context, client *redisv9.Client) error {
			return resetSessions(ctx, client)
		})
	}
	return resetSessions(ctx, s.conn)
}

func resetSessions(ctx context.Context, conn redisv9.UniversalClient) error {
	var cursor uint64
	for {
		keys, next, err := conn.Scan(ctx, cursor, sessionKeyPrefix+"*", resetScanCount).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := conn.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		if next == 0 {
			return nil
		}
		cursor = next
	}
}

func (s *storage) Close() error {
	// The host owns the Redis client and may use it after session shutdown.
	return nil
}
