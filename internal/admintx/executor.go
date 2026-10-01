// Package admintx joins goadmin-owned projections to GoAuth's managed SQL transaction.
package admintx

import (
	"context"
	"database/sql"
	"errors"

	"github.com/assurrussa/goauth/postgres"
	"github.com/georgysavva/scany/v2/sqlscan"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/assurrussa/goadmin/infrastructure/outbox"
)

type (
	key     struct{}
	managed struct {
		executor postgres.SQLExecutor
		pool     *pgxpool.Pool
	}
)

func WithExecutor(ctx context.Context, executor postgres.SQLExecutor, pool ...*pgxpool.Pool) context.Context {
	var expected *pgxpool.Pool
	if len(pool) > 0 {
		expected = pool[0]
	}
	return context.WithValue(ctx, key{}, managed{executor: executor, pool: expected})
}

func Executor(ctx context.Context) (postgres.SQLExecutor, bool) {
	e, ok := ctx.Value(key{}).(managed)
	return e.executor, ok
}

func normalize(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return pgx.ErrNoRows
	}
	return err
}

// Engine retains the host's native pgx behavior outside a managed command.
// Its SQL methods join the executor installed by the canonical transaction.
type Engine struct{ outbox.StoragePgsqlDBEngine }

func Wrap(db outbox.StoragePgsqlDBEngine) Engine { return Engine{db} }
func (e Engine) checkPool(ctx context.Context) error {
	m, ok := ctx.Value(key{}).(managed)
	if ok && m.pool != nil && e.Pool() != m.pool {
		return errors.New("admin transaction database mismatch")
	}
	return nil
}

func (e Engine) ScanOne(ctx context.Context, op string, dest any, query string, args ...any) error {
	if executor, ok := Executor(ctx); ok {
		if err := e.checkPool(ctx); err != nil {
			return err
		}
		return normalize(sqlscan.Get(ctx, executor, dest, query, args...))
	}
	return e.StoragePgsqlDBEngine.ScanOne(ctx, op, dest, query, args...)
}

func (e Engine) ScanAll(ctx context.Context, op string, dest any, query string, args ...any) error {
	if executor, ok := Executor(ctx); ok {
		if err := e.checkPool(ctx); err != nil {
			return err
		}
		return normalize(sqlscan.Select(ctx, executor, dest, query, args...))
	}
	return e.StoragePgsqlDBEngine.ScanAll(ctx, op, dest, query, args...)
}

func (e Engine) ScanOnex(ctx context.Context, op string, dest any, b outbox.StoragePgsqlSqlizer) error {
	if _, ok := Executor(ctx); !ok {
		return e.StoragePgsqlDBEngine.ScanOnex(ctx, op, dest, b)
	}
	q, args, err := b.ToSql()
	if err != nil {
		return err
	}
	return e.ScanOne(ctx, op, dest, q, args...)
}

func (e Engine) ScanAllx(ctx context.Context, op string, dest any, b outbox.StoragePgsqlSqlizer) error {
	if _, ok := Executor(ctx); !ok {
		return e.StoragePgsqlDBEngine.ScanAllx(ctx, op, dest, b)
	}
	q, args, err := b.ToSql()
	if err != nil {
		return err
	}
	return e.ScanAll(ctx, op, dest, q, args...)
}

func (e Engine) Getx(ctx context.Context, op string, dest any, b outbox.StoragePgsqlSqlizer) error {
	if _, ok := Executor(ctx); !ok {
		return e.StoragePgsqlDBEngine.Getx(ctx, op, dest, b)
	}
	return e.ScanOnex(ctx, op, dest, b)
}

// Result exposes only the row count preserved by both pgx and database/sql.
// The managed SQL executor cannot preserve PostgreSQL command tags.
type Result struct{ rowsAffected int64 }

func (r Result) RowsAffected() int64 { return r.rowsAffected }

func (e Engine) Exec(ctx context.Context, op, query string, args ...any) (Result, error) {
	if executor, ok := Executor(ctx); ok {
		if err := e.checkPool(ctx); err != nil {
			return Result{}, err
		}
		result, err := executor.ExecContext(ctx, query, args...)
		if err != nil {
			return Result{}, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return Result{}, err
		}
		return Result{rowsAffected: count}, nil
	}
	tag, err := e.StoragePgsqlDBEngine.Exec(ctx, op, query, args...)
	return Result{rowsAffected: tag.RowsAffected()}, err
}

func (e Engine) Execx(ctx context.Context, op string, b outbox.StoragePgsqlSqlizer) (Result, error) {
	if _, ok := Executor(ctx); !ok {
		tag, err := e.StoragePgsqlDBEngine.Execx(ctx, op, b)
		return Result{rowsAffected: tag.RowsAffected()}, err
	}
	q, args, err := b.ToSql()
	if err != nil {
		return Result{}, err
	}
	return e.Exec(ctx, op, q, args...)
}
