package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/postgres"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/assurrussa/goadmin/internal/admintx"
)

// InSubjectRoleTransaction protects read/check/write role mutations with the
// canonical subject locks. Callbacks must use the supplied context and must not
// send a success response until the outer transaction has committed.
func (s *RoleService) InSubjectRoleTransaction(ctx context.Context, subjects []string, fn func(context.Context) error) error {
	if s == nil || s.roleTransaction == nil {
		return errors.New("goadmin roles transaction is required")
	}
	ids := slices.Clone(subjects)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	parsed := make([]goauth.SubjectID, 0, len(ids))
	for _, id := range ids {
		subject, err := goauth.ParseSubjectID(id)
		if err != nil {
			return ErrInvalidSubjectID
		}
		parsed = append(parsed, subject)
	}
	return s.roleTransaction(ctx, parsed, fn)
}

// InRoleEditTransaction orders an editor snapshot or compare-and-write against
// GoAuth's role metadata and permission writers on the canonical role row.
func (s *RoleService) InRoleEditTransaction(ctx context.Context, roleID int64, fn func(context.Context) error) error {
	if s == nil || s.roleEditTx == nil {
		return errors.New("goadmin role editor transaction is required")
	}
	return s.roleEditTx(ctx, roleID, fn)
}

func configureRoleTransaction(service *RoleService, runtime *postgres.Runtime, pool *pgxpool.Pool) error {
	accounts, err := postgres.NewStore(runtime.Database())
	if err != nil {
		return err
	}
	service.roleTransaction = func(ctx context.Context, subjects []goauth.SubjectID, fn func(context.Context) error) error {
		return runtime.InAuthTransaction(ctx, func(txCtx context.Context) error {
			executor, err := runtime.SQLExecutor(txCtx)
			if err != nil {
				return err
			}
			txCtx = admintx.WithExecutor(txCtx, executor, pool)
			// Actor and target use one deterministic order, including reciprocal edits.
			// Lock before reading roles; ReplaceSubjectRoles alone locks too late.
			for _, subject := range subjects {
				if _, err := accounts.LockAccount(txCtx, subject); err != nil {
					return err
				}
			}
			return fn(txCtx)
		})
	}
	service.roleEditTx = func(ctx context.Context, roleID int64, fn func(context.Context) error) error {
		return runtime.InOwnedAuthTransaction(ctx, func(txCtx context.Context) error {
			executor, err := runtime.SQLExecutor(txCtx)
			if err != nil {
				return err
			}
			var locked int64
			err = executor.QueryRowContext(txCtx, `SELECT id FROM auth_roles WHERE id = $1 FOR UPDATE`, roleID).Scan(&locked)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrRoleNotFound
			}
			if err != nil {
				return fmt.Errorf("lock canonical role for editing: %w", err)
			}
			return fn(admintx.WithExecutor(txCtx, executor, pool))
		})
	}
	return nil
}
