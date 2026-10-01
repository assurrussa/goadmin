package firstadminsetup

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/internal/admintx"
)

const setupAdvisoryLockID int64 = 727250692006013788

type PostgresStore struct {
	db outbox.StoragePgsqlClient
	tx outbox.StoragePgsqlTxManager
}

func NewPostgresStore(
	db outbox.StoragePgsqlClient,
	tx outbox.StoragePgsqlTxManager,
) (*PostgresStore, error) {
	if db == nil {
		return nil, errors.New("first admin setup database is required")
	}
	if tx == nil {
		return nil, errors.New("first admin setup transaction manager is required")
	}

	return &PostgresStore{db: db, tx: tx}, nil
}

func (s *PostgresStore) Issue(
	ctx context.Context,
	tokenHash [sha256.Size]byte,
	_ time.Time,
	ttl time.Duration,
) error {
	return s.tx.RunInTx(ctx, func(txCtx context.Context) error {
		if err := s.lock(txCtx); err != nil {
			return err
		}

		var adminExists bool
		if err := admintx.Wrap(s.db.DB()).ScanOne(
			txCtx,
			"first_admin_setup.admin_exists",
			&adminExists,
			"SELECT EXISTS (SELECT 1 FROM administrations)",
		); err != nil {
			return fmt.Errorf("check first admin setup state: %w", outbox.ErrorTransform(err))
		}
		if adminExists {
			return ErrClosed
		}

		if _, err := admintx.Wrap(s.db.DB()).Exec(
			txCtx,
			"first_admin_setup.revoke_previous",
			`UPDATE goadmin_first_admin_setup_tokens
SET revoked_at = NOW()
WHERE consumed_at IS NULL AND revoked_at IS NULL`,
		); err != nil {
			return fmt.Errorf("revoke previous first admin setup tokens: %w", outbox.ErrorTransform(err))
		}

		if _, err := admintx.Wrap(s.db.DB()).Exec(
			txCtx,
			"first_admin_setup.issue",
			`INSERT INTO goadmin_first_admin_setup_tokens (token_hash, expires_at)
VALUES ($1, NOW() + ($2 * INTERVAL '1 millisecond'))`,
			tokenHash[:],
			ttl.Milliseconds(),
		); err != nil {
			return fmt.Errorf("store first admin setup token: %w", outbox.ErrorTransform(err))
		}

		return nil
	})
}

func (s *PostgresStore) Validate(
	ctx context.Context,
	tokenHash [sha256.Size]byte,
	_ time.Time,
) error {
	var valid bool
	if err := admintx.Wrap(s.db.DB()).ScanOne(
		ctx,
		"first_admin_setup.validate",
		&valid,
		`SELECT EXISTS (
    SELECT 1
    FROM goadmin_first_admin_setup_tokens
    WHERE token_hash = $1
      AND expires_at > NOW()
      AND consumed_at IS NULL
      AND revoked_at IS NULL
      AND NOT EXISTS (SELECT 1 FROM administrations)
)`,
		tokenHash[:],
	); err != nil {
		return fmt.Errorf("validate first admin setup token: %w", outbox.ErrorTransform(err))
	}
	if !valid {
		return ErrInvalidSetupToken
	}

	return nil
}

func (s *PostgresStore) Consume(
	ctx context.Context,
	tokenHash [sha256.Size]byte,
	_ time.Time,
) error {
	if err := s.lock(ctx); err != nil {
		return err
	}

	tag, err := admintx.Wrap(s.db.DB()).Exec(
		ctx,
		"first_admin_setup.consume",
		`UPDATE goadmin_first_admin_setup_tokens
SET consumed_at = NOW()
WHERE token_hash = $1
  AND expires_at > NOW()
  AND consumed_at IS NULL
  AND revoked_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM administrations)`,
		tokenHash[:],
	)
	if err != nil {
		return fmt.Errorf("consume first admin setup token: %w", outbox.ErrorTransform(err))
	}
	if tag.RowsAffected() != 1 {
		return ErrInvalidSetupToken
	}

	return nil
}

func (s *PostgresStore) ConsumeStatic(ctx context.Context) error {
	if err := s.lock(ctx); err != nil {
		return err
	}

	var adminExists bool
	if err := admintx.Wrap(s.db.DB()).ScanOne(
		ctx,
		"first_admin_setup.consume_static",
		&adminExists,
		"SELECT EXISTS (SELECT 1 FROM administrations)",
	); err != nil {
		return fmt.Errorf("check static first admin setup state: %w", outbox.ErrorTransform(err))
	}
	if adminExists {
		return ErrClosed
	}

	return nil
}

func (s *PostgresStore) Revoke(
	ctx context.Context,
	tokenHash [sha256.Size]byte,
	_ time.Time,
) error {
	if _, err := admintx.Wrap(s.db.DB()).Exec(
		ctx,
		"first_admin_setup.revoke",
		`UPDATE goadmin_first_admin_setup_tokens
SET revoked_at = NOW()
WHERE token_hash = $1 AND consumed_at IS NULL AND revoked_at IS NULL`,
		tokenHash[:],
	); err != nil {
		return fmt.Errorf("revoke first admin setup token: %w", outbox.ErrorTransform(err))
	}

	return nil
}

func (s *PostgresStore) lock(ctx context.Context) error {
	if _, err := admintx.Wrap(s.db.DB()).Exec(
		ctx,
		"first_admin_setup.lock",
		"SELECT pg_advisory_xact_lock($1)",
		setupAdvisoryLockID,
	); err != nil {
		return fmt.Errorf("lock first admin setup: %w", outbox.ErrorTransform(err))
	}

	return nil
}
