//go:build integration

package firstadminsetup

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	identity "github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/tests"
)

func TestPostgresStoreTokenLifecycleAndTransactionRollback(t *testing.T) {
	ctx := context.Background()
	db, _, cleanup := tests.PrepareDB(
		ctx,
		t,
		"TestFirstAdminSetupTokenLifecycle",
		tests.WithDatabasePathFilesMigration("migrations/sql"),
	)
	t.Cleanup(func() { cleanup(ctx) })
	tx := outbox.PgsqlTrxNew(db.DB())
	store, err := NewPostgresStore(db, tx)
	require.NoError(t, err)

	now := time.Date(2026, time.July, 10, 12, 0, 0, 0, time.UTC)
	service, err := New(
		store,
		WithClock(func() time.Time { return now }),
	)
	require.NoError(t, err)
	token, err := service.Issue(ctx)
	require.NoError(t, err)

	var storedHash []byte
	require.NoError(t, db.DB().ScanOne(
		ctx,
		"test.first_admin_setup.hash",
		&storedHash,
		"SELECT token_hash FROM goadmin_first_admin_setup_tokens",
	))
	require.Equal(t, sha256.Size, len(storedHash))
	expectedHash := sha256.Sum256([]byte(token))
	require.Equal(t, expectedHash[:], storedHash)

	rollback := errors.New("rollback setup")
	err = tx.RunInTx(ctx, func(txCtx context.Context) error {
		require.NoError(t, service.Consume(txCtx, token))
		return rollback
	})
	require.ErrorIs(t, err, rollback)
	require.NoError(t, service.Validate(ctx, token))

	require.NoError(t, tx.RunInTx(ctx, func(txCtx context.Context) error {
		return service.Consume(txCtx, token)
	}))
	require.ErrorIs(t, service.Consume(ctx, token), ErrInvalidSetupToken)
}

func TestPostgresStoreRefusesIssueAfterAnyAdminHasExisted(t *testing.T) {
	ctx := context.Background()
	db, _, cleanup := tests.PrepareDB(
		ctx,
		t,
		"TestFirstAdminSetupClosed",
		tests.WithDatabasePathFilesMigration("migrations/sql"),
	)
	t.Cleanup(func() { cleanup(ctx) })
	tx := outbox.PgsqlTrxNew(db.DB())
	store, err := NewPostgresStore(db, tx)
	require.NoError(t, err)
	service, err := New(store)
	require.NoError(t, err)

	err = insertTestAdmin(ctx, db)
	require.NoError(t, err)

	_, err = service.Issue(ctx)
	require.ErrorIs(t, err, ErrClosed)
}

func TestPostgresStoreStaticTokenRollbackAndClose(t *testing.T) {
	ctx := context.Background()
	db, _, cleanup := tests.PrepareDB(
		ctx,
		t,
		"TestFirstAdminStaticSetup",
		tests.WithDatabasePathFilesMigration("migrations/sql"),
	)
	t.Cleanup(func() { cleanup(ctx) })
	tx := outbox.PgsqlTrxNew(db.DB())
	store, err := NewPostgresStore(db, tx)
	require.NoError(t, err)

	raw := strings.Repeat("static-setup-secret-", 2)
	service, err := New(store, WithStaticTokenHash(sha256.Sum256([]byte(raw))))
	require.NoError(t, err)

	errRollback := errors.New("rollback static setup")
	err = tx.RunInTx(ctx, func(txCtx context.Context) error {
		require.NoError(t, service.Consume(txCtx, raw))
		return errRollback
	})
	require.ErrorIs(t, err, errRollback)

	require.NoError(t, tx.RunInTx(ctx, func(txCtx context.Context) error {
		if err := service.Consume(txCtx, raw); err != nil {
			return err
		}
		return insertTestAdmin(txCtx, db)
	}))

	err = tx.RunInTx(ctx, func(txCtx context.Context) error {
		return service.Consume(txCtx, raw)
	})
	require.ErrorIs(t, err, ErrClosed)
}

func TestPostgresStoreStaticTokenSerializesFirstAdminCreation(t *testing.T) {
	ctx := context.Background()
	db, _, cleanup := tests.PrepareDB(
		ctx,
		t,
		"TestFirstAdminStaticSetupConcurrency",
		tests.WithDatabasePathFilesMigration("migrations/sql"),
	)
	t.Cleanup(func() { cleanup(ctx) })
	tx := outbox.PgsqlTrxNew(db.DB())
	store, err := NewPostgresStore(db, tx)
	require.NoError(t, err)

	raw := strings.Repeat("concurrent-static-secret-", 2)
	service, err := New(store, WithStaticTokenHash(sha256.Sum256([]byte(raw))))
	require.NoError(t, err)

	firstReady := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstResult := make(chan error, 1)
	go func() {
		firstResult <- tx.RunInTx(ctx, func(txCtx context.Context) error {
			if err := service.Consume(txCtx, raw); err != nil {
				return err
			}
			if err := insertTestAdmin(txCtx, db); err != nil {
				return err
			}
			close(firstReady)
			<-releaseFirst
			return nil
		})
	}()
	select {
	case err := <-firstResult:
		require.NoError(t, err)
		return
	case <-firstReady:
	}

	secondResult := make(chan error, 1)
	go func() {
		secondResult <- tx.RunInTx(ctx, func(txCtx context.Context) error {
			return service.Consume(txCtx, raw)
		})
	}()

	select {
	case err := <-secondResult:
		close(releaseFirst)
		require.NoError(t, <-firstResult)
		t.Fatalf("second registration did not wait for the first transaction: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseFirst)
	require.NoError(t, <-firstResult)
	require.ErrorIs(t, <-secondResult, ErrClosed)
}

func TestPostgresStoreGeneratedTokensRecheckAdminStateAfterAdvisoryLock(t *testing.T) {
	ctx := context.Background()
	db, _, cleanup := tests.PrepareDB(
		ctx,
		t,
		"TestFirstAdminGeneratedSetupConcurrency",
		tests.WithDatabasePathFilesMigration("migrations/sql"),
	)
	t.Cleanup(func() { cleanup(ctx) })
	tx := outbox.PgsqlTrxNew(db.DB())
	store, err := NewPostgresStore(db, tx)
	require.NoError(t, err)

	firstHash := sha256.Sum256([]byte("first-generated-setup-token"))
	secondHash := sha256.Sum256([]byte("second-generated-setup-token"))
	for _, tokenHash := range [][sha256.Size]byte{firstHash, secondHash} {
		_, err = db.DB().Exec(
			ctx,
			"test.first_admin_setup.generated_token",
			`INSERT INTO goadmin_first_admin_setup_tokens (token_hash, expires_at)
VALUES ($1, NOW() + INTERVAL '1 hour')`,
			tokenHash[:],
		)
		require.NoError(t, err)
	}

	insertAdmin := func(txCtx context.Context) error {
		return insertTestAdmin(txCtx, db)
	}

	firstReady := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstResult := make(chan error, 1)
	go func() {
		firstResult <- tx.RunInTx(ctx, func(txCtx context.Context) error {
			if err := store.Consume(txCtx, firstHash, time.Now()); err != nil {
				return err
			}
			if err := insertAdmin(txCtx); err != nil {
				return err
			}
			close(firstReady)
			<-releaseFirst
			return nil
		})
	}()
	select {
	case err := <-firstResult:
		require.NoError(t, err)
		t.Fatal("first registration finished before it was released")
	case <-firstReady:
	}

	secondResult := make(chan error, 1)
	go func() {
		secondResult <- tx.RunInTx(ctx, func(txCtx context.Context) error {
			if err := store.Consume(txCtx, secondHash, time.Now()); err != nil {
				return err
			}

			return insertAdmin(txCtx)
		})
	}()

	select {
	case err := <-secondResult:
		close(releaseFirst)
		require.NoError(t, <-firstResult)
		t.Fatalf("second registration did not wait for the first transaction: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseFirst)
	require.NoError(t, <-firstResult)
	require.ErrorIs(t, <-secondResult, ErrInvalidSetupToken)

	var adminCount int
	require.NoError(t, db.DB().ScanOne(
		ctx,
		"test.first_admin_setup.generated_admin_count",
		&adminCount,
		"SELECT count(*) FROM administrations",
	))
	require.Equal(t, 1, adminCount)
}

func insertTestAdmin(ctx context.Context, db outbox.StoragePgsqlClient) error {
	subjectID := identity.NewUserID()
	if _, err := db.DB().Exec(ctx, "test.first_admin_setup.subject",
		`INSERT INTO auth_subjects (id, status, created_at, updated_at)
VALUES ($1, 'active', NOW(), NOW())`, subjectID); err != nil {
		return err
	}
	_, err := db.DB().Exec(ctx, "test.first_admin_setup.admin",
		`INSERT INTO administrations (subject_id, uuid) VALUES ($1, $2)`, subjectID, identity.NewUserID())
	return err
}
