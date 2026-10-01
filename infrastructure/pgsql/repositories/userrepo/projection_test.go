package userrepo //nolint:testpackage // required

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	datagrid "github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	authcore "github.com/assurrussa/goadmin/internal/auth"
	identity "github.com/assurrussa/goadmin/internal/identity"
)

func TestGetByUUIDReturnsCanonicalIdentityWithUserProjection(t *testing.T) {
	t.Parallel()

	userID := identity.NewUserID()
	repo := Must(NewOptions(&fakeProjectionClient{
		db: &fakeProjectionDB{
			scanOnexFn: func(_ context.Context, op string, dest any, sqlizer outbox.StoragePgsqlSqlizer) error {
				require.Equal(t, "user.repo.GetByUUID", op)
				query, _, err := sqlizer.ToSql()
				require.NoError(t, err)
				require.Contains(t, query, "FROM users u")
				require.Contains(t, query, "JOIN auth_subjects s ON s.id = u.subject_id")
				require.Contains(t, query, "JOIN auth_identifiers i ON i.subject_id = u.subject_id")
				require.Contains(t, query, "LEFT JOIN auth_basic_profiles bp ON bp.subject_id = u.subject_id")
				require.Contains(t, query, "i.display_value AS email")
				require.NotContains(t, query, "auth_local_credentials")
				require.NotContains(t, query, "u.email")
				require.NotContains(t, query, "u.password_hash")

				profile, ok := dest.(*authcore.Profile)
				require.True(t, ok)
				*profile = authcore.Profile{
					ID:       19,
					PublicID: userID,
					Email:    "canonical@example.com",
				}

				return nil
			},
		},
	}, fakeProjectionTxManager{}))

	got, err := repo.GetByUUID(context.Background(), userID)
	require.NoError(t, err)
	require.Equal(t, int64(19), got.ID)
	require.Equal(t, "canonical@example.com", got.Email)
}

func TestGetListUsesCanonicalIdentityColumns(t *testing.T) {
	t.Parallel()

	repo := Must(NewOptions(&fakeProjectionClient{
		db: &fakeProjectionDB{
			scanOnexFn: func(_ context.Context, op string, dest any, sqlizer outbox.StoragePgsqlSqlizer) error {
				require.Equal(t, "user.repo.GetList", op)
				query, _, err := sqlizer.ToSql()
				require.NoError(t, err)
				require.Contains(t, query, "FROM users u")
				require.Contains(t, query, "JOIN auth_subjects s ON s.id = u.subject_id")
				require.Contains(t, query, "i.verified_at IS NOT NULL")
				count, ok := dest.(*int)
				require.True(t, ok)
				*count = 1
				return nil
			},
			scanAllxFn: func(_ context.Context, op string, dest any, sqlizer outbox.StoragePgsqlSqlizer) error {
				require.Equal(t, "user.repo.GetList", op)
				query, _, err := sqlizer.ToSql()
				require.NoError(t, err)
				require.Contains(t, query, "FROM users u")
				require.Contains(t, query, "JOIN auth_subjects s ON s.id = u.subject_id")
				require.Contains(t, query, "JOIN auth_identifiers i ON i.subject_id = u.subject_id")
				require.Contains(t, query, "LEFT JOIN auth_basic_profiles bp ON bp.subject_id = u.subject_id")
				require.Contains(t, query, "i.display_value AS email")
				require.NotContains(t, query, "u.email")
				require.NotContains(t, query, "auth_local_credentials")
				profiles, ok := dest.(*[]authcore.Profile)
				require.True(t, ok)
				*profiles = []authcore.Profile{{Email: "canonical@example.com"}}
				return nil
			},
		},
	}, fakeProjectionTxManager{}))

	got, total, err := repo.GetList(context.Background(), datagrid.Filters{
		Fields: map[string]any{"emailStatus": "confirmed"},
	})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, "canonical@example.com", got[0].Email)
}

func TestUpdateWritesOnlyProjectionColumns(t *testing.T) {
	t.Parallel()

	repo := Must(NewOptions(&fakeProjectionClient{
		db: &fakeProjectionDB{
			execxFn: func(_ context.Context, op string, sqlizer outbox.StoragePgsqlSqlizer) (pgconn.CommandTag, error) {
				require.Equal(t, "user.repo.Update", op)
				query, _, err := sqlizer.ToSql()
				require.NoError(t, err)
				require.Contains(t, query, "UPDATE users")
				require.Contains(t, query, "bio")
				require.Contains(t, query, "data")
				require.Contains(t, query, "version")
				require.NotContains(t, query, "username")
				require.NotContains(t, query, "name")
				require.NotContains(t, query, "last_name")
				require.NotContains(t, query, "father_name")
				require.NotContains(t, query, "birthday")
				require.NotContains(t, query, "gender")

				return pgconn.NewCommandTag("UPDATE 1"), nil
			},
		},
	}, fakeProjectionTxManager{}))

	err := repo.Update(context.Background(), 19, authcore.Profile{Name: "Canonical"})
	require.NoError(t, err)
}

type fakeProjectionClient struct {
	db *fakeProjectionDB
}

func (c *fakeProjectionClient) DB() outbox.StoragePgsqlDBEngine { return c.db }
func (c *fakeProjectionClient) Close() error                    { return nil }

type fakeProjectionTxManager struct{}

func (fakeProjectionTxManager) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func (fakeProjectionTxManager) ReadCommitted(context.Context, pgx.TxAccessMode, outbox.StoragePgsqlFnCallback) error {
	return nil
}

func (fakeProjectionTxManager) RepeatableRead(context.Context, pgx.TxAccessMode, outbox.StoragePgsqlFnCallback) error {
	return nil
}

func (fakeProjectionTxManager) Serializable(context.Context, pgx.TxAccessMode, outbox.StoragePgsqlFnCallback) error {
	return nil
}

type fakeProjectionDB struct {
	scanOnexFn func(context.Context, string, any, outbox.StoragePgsqlSqlizer) error
	scanAllxFn func(context.Context, string, any, outbox.StoragePgsqlSqlizer) error
	getxFn     func(context.Context, string, any, outbox.StoragePgsqlSqlizer) error
	execxFn    func(context.Context, string, outbox.StoragePgsqlSqlizer) (pgconn.CommandTag, error)
}

func (db *fakeProjectionDB) ScanOne(context.Context, string, any, string, ...any) error {
	return errors.New("unexpected ScanOne call")
}

func (db *fakeProjectionDB) ScanAll(context.Context, string, any, string, ...any) error {
	return errors.New("unexpected ScanAll call")
}

func (db *fakeProjectionDB) ScanOnex(ctx context.Context, op string, dest any, sqlizer outbox.StoragePgsqlSqlizer) error {
	if db.scanOnexFn == nil {
		return pgx.ErrNoRows
	}

	return db.scanOnexFn(ctx, op, dest, sqlizer)
}

func (db *fakeProjectionDB) ScanAllx(ctx context.Context, op string, dest any, sqlizer outbox.StoragePgsqlSqlizer) error {
	if db.scanAllxFn == nil {
		return errors.New("unexpected ScanAllx call")
	}

	return db.scanAllxFn(ctx, op, dest, sqlizer)
}

func (db *fakeProjectionDB) QueryRow(context.Context, string, string, ...any) pgx.Row { return nil }

func (db *fakeProjectionDB) Query(context.Context, string, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("unexpected Query call")
}

func (db *fakeProjectionDB) Exec(context.Context, string, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag(""), errors.New("unexpected Exec call")
}

func (db *fakeProjectionDB) Getx(ctx context.Context, op string, dest any, sqlizer outbox.StoragePgsqlSqlizer) error {
	if db.getxFn == nil {
		return errors.New("unexpected Getx call")
	}

	return db.getxFn(ctx, op, dest, sqlizer)
}

func (db *fakeProjectionDB) Selectx(context.Context, string, any, outbox.StoragePgsqlSqlizer) error {
	return errors.New("unexpected Selectx call")
}

func (db *fakeProjectionDB) Execx(ctx context.Context, op string, sqlizer outbox.StoragePgsqlSqlizer) (pgconn.CommandTag, error) {
	if db.execxFn == nil {
		return pgconn.NewCommandTag(""), errors.New("unexpected Execx call")
	}

	return db.execxFn(ctx, op, sqlizer)
}

func (db *fakeProjectionDB) Queryx(context.Context, string, outbox.StoragePgsqlSqlizer) (pgx.Rows, error) {
	return nil, errors.New("unexpected Queryx call")
}

func (db *fakeProjectionDB) SendBatch(context.Context, string, *pgx.Batch) pgx.BatchResults {
	return nil
}

func (db *fakeProjectionDB) CopyFrom(context.Context, string, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("unexpected CopyFrom call")
}

func (db *fakeProjectionDB) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) { return nil, nil } //nolint:nilnil,lll // required
func (db *fakeProjectionDB) Ping(context.Context) error                             { return nil }
func (db *fakeProjectionDB) Close()                                                 {}
func (db *fakeProjectionDB) Pool() *pgxpool.Pool                                    { return nil }

var _ outbox.StoragePgsqlDBEngine = (*fakeProjectionDB)(nil)
