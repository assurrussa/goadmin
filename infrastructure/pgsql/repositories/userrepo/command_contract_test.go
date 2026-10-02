package userrepo //nolint:testpackage // verifies the repository's managed SQL boundary

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	"github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/internal/admintx"
	authcore "github.com/assurrussa/goadmin/internal/auth"
)

type userCommandExecutor struct {
	query    string
	args     []any
	calls    int
	queryErr error
}

func (e *userCommandExecutor) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	e.query = query
	e.args = args
	e.calls++
	return driver.RowsAffected(1), nil
}

func (e *userCommandExecutor) QueryContext(_ context.Context, query string, args ...any) (*sql.Rows, error) {
	e.query = query
	e.args = args
	e.calls++
	return nil, e.queryErr
}

func (*userCommandExecutor) QueryRowContext(context.Context, string, ...any) *sql.Row {
	panic("unexpected QueryRowContext")
}

func TestUserCommandUsesManagedSQLExecutor(t *testing.T) {
	t.Parallel()
	native := &fakeProjectionDB{
		scanOnexFn: func(context.Context, string, any, outbox.StoragePgsqlSqlizer) error {
			t.Fatal("escaped managed read")
			return nil
		},
	}
	repo := Must(NewOptions(&fakeProjectionClient{db: native}, fakeProjectionTxManager{}))
	failure := errors.New("managed read sentinel")
	executor := &userCommandExecutor{queryErr: failure}
	ctx := admintx.WithExecutor(t.Context(), executor)
	_, err := repo.GetByID(ctx, 42)
	require.ErrorIs(t, err, failure)
	require.Equal(t, 1, executor.calls)
	require.Contains(t, executor.query, "WHERE u.id = $1")
	require.Equal(t, []any{int64(42)}, executor.args)
	require.NoError(t, repo.Update(ctx, 42, authcore.Profile{}))
	require.Equal(t, 2, executor.calls)
	require.Contains(t, executor.query, "UPDATE users SET")
	require.Contains(t, executor.query, "version = version + 1")
	require.Equal(t, int64(42), executor.args[len(executor.args)-1])
}

func TestUserListCountAndRowsPreserveBoundSearchAndSort(t *testing.T) {
	t.Parallel()
	for key, column := range sortColumns {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			var countQuery, listQuery string
			var countArgs, listArgs []any
			repo := Must(NewOptions(&fakeProjectionClient{db: &fakeProjectionDB{
				scanOnexFn: func(_ context.Context, _ string, dest any, b outbox.StoragePgsqlSqlizer) error {
					var err error
					countQuery, countArgs, err = b.ToSql()
					require.NoError(t, err)
					p, ok := dest.(*int)
					require.True(t, ok)
					*p = 0
					return nil
				},
				scanAllxFn: func(_ context.Context, _ string, _ any, b outbox.StoragePgsqlSqlizer) error {
					var err error
					listQuery, listArgs, err = b.ToSql()
					require.NoError(t, err)
					return nil
				},
			}}, fakeProjectionTxManager{}))
			search := "O'Reilly"
			_, _, err := repo.GetList(t.Context(), datagrid.Filters{
				Page: 1, Limit: 10000, Search: search, SortBy: key, SortOrder: "desc",
				Fields: map[string]any{"status": "active", "username": "reader"},
			})
			require.NoError(t, err)
			for _, query := range []string{countQuery, listQuery} {
				require.NotContains(t, query, search)
				require.Contains(t, query, "i.normalized_value ILIKE")
				require.Contains(t, query, "bp.given_name ILIKE")
				require.Contains(t, query, "bp.family_name ILIKE")
				require.Contains(t, query, "bp.username LIKE")
			}
			require.Equal(t, countArgs, listArgs)
			require.Contains(t, countArgs, "%O'Reilly%")
			require.Contains(t, listQuery, "ORDER BY "+column+" desc")
			require.Contains(t, listQuery, "LIMIT 10000 OFFSET 0")
		})
	}
}

func TestUserSortRejectsUnknownColumnAndOrder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		field, order string
		want         string
	}{
		{"name; DROP TABLE users", "asc", ""},
		{"name", "desc; DROP TABLE users", "ORDER BY bp.given_name asc"},
	} {
		query, _, err := applyUserSort(outbox.BuilderDollar().Select("u.id").From("users u"), datagrid.Filters{
			SortBy: tc.field, SortOrder: tc.order,
		}).ToSql()
		require.NoError(t, err)
		require.NotContains(t, query, "DROP")
		if tc.want == "" {
			require.NotContains(t, query, "ORDER BY")
		} else {
			require.Contains(t, query, tc.want)
		}
	}
}
