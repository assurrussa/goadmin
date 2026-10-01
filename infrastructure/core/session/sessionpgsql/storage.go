// Package sessionpgsql persists Fiber browser sessions in host-owned PostgreSQL.
package sessionpgsql

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/gob"
	"errors"
	"time"

	"github.com/gofiber/fiber/v3"
)

// Storage borrows its database. Construction does not start maintenance workers.
type Storage struct{ db *sql.DB }

var _ fiber.Storage = (*Storage)(nil)

// New creates storage backed by an existing, host-owned database handle.
func New(db *sql.DB) (*Storage, error) {
	if db == nil {
		return nil, errors.New("PostgreSQL session database is required")
	}
	return &Storage{db: db}, nil
}

// Get reads a non-expired session payload.
func (s *Storage) Get(key string) ([]byte, error) { return s.GetWithContext(context.Background(), key) }

// GetWithContext reads a non-expired, Fiber-compatible session payload.
func (s *Storage) GetWithContext(ctx context.Context, key string) ([]byte, error) {
	if key == "" {
		return nil, nil
	}
	var value []byte
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM goadmin_browser_sessions
WHERE id = $1 AND (expires_at IS NULL OR expires_at > statement_timestamp())`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// Match Fiber's gob decoder and discard only the observed incompatible value.
	// A concurrent valid replacement must survive this compatibility cleanup.
	var decoded map[any]any
	if err := gob.NewDecoder(bytes.NewReader(value)).Decode(&decoded); err != nil {
		_, err = s.db.ExecContext(ctx, `DELETE FROM goadmin_browser_sessions WHERE id = $1 AND payload = $2`, key, value)
		return nil, err
	}
	return value, nil
}

// Set saves a session payload with its storage lifetime.
func (s *Storage) Set(key string, value []byte, expiry time.Duration) error {
	return s.SetWithContext(context.Background(), key, value, expiry)
}

// SetWithContext atomically replaces a payload and its expiry.
func (s *Storage) SetWithContext(ctx context.Context, key string, value []byte, expiry time.Duration) error {
	if key == "" || len(value) == 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO goadmin_browser_sessions (id, payload, expires_at)
VALUES ($1, $2, CASE WHEN $3::double precision > 0
THEN statement_timestamp() + $3::double precision * interval '1 second' ELSE NULL END)
ON CONFLICT (id) DO UPDATE SET payload = EXCLUDED.payload, expires_at = EXCLUDED.expires_at`, key, value, expiry.Seconds())
	return err
}

// Delete removes one session.
func (s *Storage) Delete(key string) error { return s.DeleteWithContext(context.Background(), key) }

// DeleteWithContext removes one session with caller cancellation.
func (s *Storage) DeleteWithContext(ctx context.Context, key string) error {
	if key == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM goadmin_browser_sessions WHERE id = $1`, key)
	return err
}

// Reset removes all browser sessions owned by this subsystem.
func (s *Storage) Reset() error { return s.ResetWithContext(context.Background()) }

// ResetWithContext removes only goadmin browser session payloads.
func (s *Storage) ResetWithContext(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM goadmin_browser_sessions`)
	return err
}

// Cleanup removes one bounded batch of expired records. Multiple hosts may run it.
func (s *Storage) Cleanup(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `WITH expired AS (
SELECT id FROM goadmin_browser_sessions WHERE expires_at <= statement_timestamp()
ORDER BY expires_at LIMIT 500 FOR UPDATE SKIP LOCKED
) DELETE FROM goadmin_browser_sessions AS sessions USING expired WHERE sessions.id = expired.id`)
	return err
}

// Close leaves the borrowed database open for the host and other subsystems.
func (s *Storage) Close() error { return nil }
