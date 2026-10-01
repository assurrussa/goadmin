package admintx //nolint:testpackage // verifies private engine and executor bridge

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/infrastructure/outbox"
)

type executorStub struct {
	result sql.Result
	err    error
	query  string
	args   []any
	calls  int
}

func (e *executorStub) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	e.calls++
	e.query, e.args = query, args
	return e.result, e.err
}

func (*executorStub) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	panic("not used")
}
func (*executorStub) QueryRowContext(context.Context, string, ...any) *sql.Row { panic("not used") }

type resultError struct{ err error }

func (r resultError) LastInsertId() (int64, error) { panic("not used") }
func (r resultError) RowsAffected() (int64, error) { return 0, r.err }

type sqlizerStub struct {
	query string
	err   error
}

//nolint:revive // implements the existing SQLizer contract
func (s sqlizerStub) ToSql() (string, []any, error) { return s.query, []any{7}, s.err }

func TestExecPreservesOnlyRowsAffected(t *testing.T) {
	for _, query := range []string{
		"INSERT INTO records(id) VALUES ($1)",
		"UPDATE records SET id=$1",
		"DELETE FROM records WHERE id=$1",
		"WITH deleted AS (DELETE FROM records RETURNING id) INSERT INTO archive SELECT id FROM deleted",
		"WITH updated AS (UPDATE records SET id=$1 RETURNING id) SELECT id FROM updated",
		"SELECT pg_advisory_xact_lock($1)",
	} {
		t.Run(query, func(t *testing.T) {
			for _, count := range []int64{0, 3} {
				executor := &executorStub{result: driver.RowsAffected(count)}
				ctx := WithExecutor(t.Context(), executor)
				result, err := (Engine{}).Exec(ctx, "test", query, 7)
				require.NoError(t, err)
				require.Equal(t, count, result.RowsAffected())
				require.Equal(t, query, executor.query)
				require.Equal(t, []any{7}, executor.args)
				require.Equal(t, 1, executor.calls)
				result, err = (Engine{}).Execx(ctx, "test", sqlizerStub{query: query})
				require.NoError(t, err)
				require.Equal(t, count, result.RowsAffected())
			}
		})
	}
}

func TestExecErrors(t *testing.T) {
	failure := errors.New("execution failed")
	for _, executor := range []*executorStub{
		{err: failure},
		{result: resultError{err: failure}},
	} {
		result, err := (Engine{}).Exec(WithExecutor(t.Context(), executor), "test", "DELETE FROM records")
		require.ErrorIs(t, err, failure)
		require.Zero(t, result.RowsAffected())
	}
	executor := &executorStub{}
	result, err := (Engine{}).Execx(WithExecutor(t.Context(), executor), "test", sqlizerStub{err: failure})
	require.ErrorIs(t, err, failure)
	require.Zero(t, result.RowsAffected())
	require.Zero(t, executor.calls)
}

type nativeEngineStub struct {
	outbox.StoragePgsqlDBEngine
	query string
	args  []any
}

func (e *nativeEngineStub) Exec(_ context.Context, _, query string, args ...any) (pgconn.CommandTag, error) {
	e.query, e.args = query, args
	return pgconn.NewCommandTag("INSERT 0 4"), nil
}

func (e *nativeEngineStub) Execx(ctx context.Context, op string, b outbox.StoragePgsqlSqlizer) (pgconn.CommandTag, error) {
	query, args, err := b.ToSql()
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	return e.Exec(ctx, op, query, args...)
}

func TestExecNativeRowsAffected(t *testing.T) {
	native := &nativeEngineStub{}
	engine := Wrap(native)
	result, err := engine.Exec(t.Context(), "test", "INSERT INTO records(id) VALUES ($1)", 7)
	require.NoError(t, err)
	require.Equal(t, int64(4), result.RowsAffected())
	require.Equal(t, []any{7}, native.args)
	result, err = engine.Execx(t.Context(), "test", sqlizerStub{query: "INSERT INTO records(id) VALUES ($1)"})
	require.NoError(t, err)
	require.Equal(t, int64(4), result.RowsAffected())
	require.Equal(t, "INSERT INTO records(id) VALUES ($1)", native.query)
}
