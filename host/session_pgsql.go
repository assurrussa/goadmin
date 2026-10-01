package host

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"

	"github.com/assurrussa/goadmin/infrastructure/core/session/sessionpgsql"
	"github.com/assurrussa/goadmin/infrastructure/core/session/sessionredis"
)

// PgSessionRuntime owns browser-session maintenance, borrowing the host database.
type PgSessionRuntime struct {
	Middleware fiber.Handler
	Store      *session.Store
	storage    *sessionpgsql.Storage
}

// NewPostgresSessions creates the admin browser session store without starting
// workers or opening connections. Migrate the database before using the store.
func NewPostgresSessions(cfg AdminConfig, db *sql.DB) (*PgSessionRuntime, error) {
	if cfg.SessionTTL < 0 || cfg.SessionInactiveTTL < 0 {
		return nil, errors.New("session durations cannot be negative")
	}
	idle := cfg.SessionInactiveTTL
	if idle == 0 {
		idle = session.ConfigDefault.IdleTimeout
	}
	if cfg.SessionTTL > 0 && cfg.SessionTTL < idle {
		return nil, errors.New("session absolute timeout must be greater than or equal to idle timeout")
	}
	storage, err := sessionpgsql.New(db)
	if err != nil {
		return nil, err
	}
	middleware, store := sessionredis.CreateAdminSessionStoreWithStorage(storage, cfg)
	return &PgSessionRuntime{Middleware: middleware, Store: store, storage: storage}, nil
}

// Run performs periodic bounded expiry cleanup until the host cancels ctx.
// The caller owns the worker goroutine and must wait for Run before shutdown.
func (r *PgSessionRuntime) Run(ctx context.Context) error {
	if r == nil || r.storage == nil {
		return errors.New("PostgreSQL session runtime is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if err := r.storage.Cleanup(ctx); err != nil {
			return fmt.Errorf("cleanup PostgreSQL browser sessions: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
