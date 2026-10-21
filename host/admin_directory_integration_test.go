//go:build integration

package host //nolint:testpackage // verifies canonical membership projection and transactional locking

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/internal/identity"
)

func TestAdminDirectoryCanonicalSearchAndLock(t *testing.T) {
	cfg, deps := assemblyFixture(t)
	runtime, err := New(t.Context(), cfg, deps)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, runtime.Close()) })
	db := runtime.Auth.Runtime().Database()
	provision := func(email string, member bool) string {
		account, e := runtime.Auth.Runtime().ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{
			Email: email, Password: assemblyPassword,
		})
		require.NoError(t, e)
		if member {
			_, e = runtime.Auth.admins.ProvisionAccount(t.Context(), account, identity.NewUserID())
			require.NoError(t, e)
		}
		return account.Subject.ID.String()
	}
	active := provision("directory-active@example.test", true)
	suspended := provision("directory-suspended@example.test", true)
	deleted := provision("directory-deleted@example.test", true)
	nonadmin := provision("directory-nonadmin@example.test", false)
	_, err = db.ExecContext(t.Context(), `UPDATE auth_subjects SET status='suspended' WHERE id=$1`, suspended)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `UPDATE administrations SET deleted_at=now() WHERE subject_id=$1`, deleted)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `UPDATE auth_basic_profiles SET given_name='Артур',family_name='Тест',username='directory_user' WHERE subject_id=$1`, active)
	require.NoError(t, err)
	d, err := NewAdminDirectory(db, directoryPermissions{}, NewPermissionKey("directory", "read"))
	require.NoError(t, err)
	actor := Actor{AdminID: 1, SubjectID: active}
	for _, query := range []string{"directory", "Арту", "directory_", "Артур Тест", "Артур Те"} {
		found, e := d.Search(t.Context(), actor, query, 20)
		require.NoError(t, e)
		require.Len(t, found.Entries, 1)
		require.Equal(t, active, found.Entries[0].SubjectID)
		require.False(t, found.More)
	}
	wild, e := d.Search(t.Context(), actor, "%%", 20)
	require.NoError(t, e)
	require.Empty(t, wild.Entries)
	assertDirectorySingleConnection(t, db, d, actor, active)
	for _, subject := range []string{suspended, deleted, nonadmin, uuid.NewString()} {
		tx, e := db.BeginTx(t.Context(), nil)
		require.NoError(t, e)
		require.ErrorIs(t, d.LockActive(t.Context(), tx, subject), ErrAdminDirectoryUnavailable)
		require.NoError(t, tx.Rollback())
	}
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	require.NoError(t, d.LockActive(t.Context(), tx, active))
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	_, err = db.ExecContext(ctx, `UPDATE auth_subjects SET status='suspended' WHERE id=$1`, active)
	require.Error(t, err, "selection must hold account status stable through host write")
	require.NoError(t, tx.Commit())
	_, err = db.ExecContext(t.Context(), `UPDATE auth_subjects SET status='suspended' WHERE id=$1`, active)
	require.NoError(t, err)
	found, err := d.Search(t.Context(), actor, "directory", 20)
	require.NoError(t, err)
	require.Empty(t, found.Entries)
}

// Models the real permission checker borrowing a connection independently.
type directoryDBPermissions struct {
	db    *sql.DB
	calls int
}

func (p *directoryDBPermissions) CheckPermission(ctx context.Context, _ string, _ PermissionKey) error {
	p.calls++
	var one int
	return p.db.QueryRowContext(ctx, "SELECT 1").Scan(&one)
}

func assertDirectorySingleConnection(t *testing.T, db *sql.DB, d *AdminDirectory, actor Actor, subject string) {
	t.Helper()
	permissions := &directoryDBPermissions{db: db}
	d.permissions = permissions
	db.SetMaxOpenConns(1)
	defer func() { db.SetMaxOpenConns(0); d.permissions = directoryPermissions{} }()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	require.NoError(t, d.authorize(ctx, actor))
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	require.NoError(t, d.LockActive(ctx, tx, subject))
	require.Equal(t, 1, permissions.calls, "no permission query may borrow a connection while transaction owns it")
	require.NoError(t, tx.Commit())
}
