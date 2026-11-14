package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/postgres"
	"github.com/assurrussa/goauth/rbac"
	pgstorage "github.com/assurrussa/outbox/backends/pgsql/storage"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/assurrussa/goadmin/internal/admintx"
	"github.com/assurrussa/goadmin/internal/authsnapshot"
)

// AuthorizationCacheConfig enables bounded admin authorization snapshots. A nil
// config preserves the database-backed policy.
type AuthorizationCacheConfig = authsnapshot.Config

type cachedMemberships struct {
	AdminMemberships
	cache   *authsnapshot.Cache
	runtime *postgres.Runtime
	pool    *pgxpool.Pool
}

func (m *cachedMemberships) HasAdminMembership(ctx context.Context, id goauth.SubjectID) (bool, error) {
	if _, managed := admintx.Executor(ctx); managed {
		return m.AdminMemberships.HasAdminMembership(ctx, id)
	}
	if m.runtime != nil {
		active, err := m.runtime.AuthTransactionActive(ctx)
		if err != nil {
			return false, err
		}
		if active {
			q, err := m.runtime.SQLExecutor(ctx)
			if err != nil {
				return false, err
			}
			return m.AdminMemberships.HasAdminMembership(admintx.WithExecutor(ctx, q, m.pool), id)
		}
	}
	v, err := m.cache.View(ctx)
	if err != nil {
		return false, err
	}
	return v.Member(id), nil
}

func loadAuthorization(runtime *postgres.Runtime) authsnapshot.Loader {
	return func(ctx context.Context) (authsnapshot.Data, error) {
		result := authsnapshot.Data{AdminIDs: make(map[int64]goauth.SubjectID)}
		snapshot, err := runtime.ScopedAuthorizationSnapshot(ctx, func(
			ctx context.Context, q postgres.SQLExecutor,
		) ([]goauth.SubjectID, error) {
			rows, err := q.QueryContext(ctx, `SELECT id, subject_id FROM administrations WHERE deleted_at IS NULL`)
			if err != nil {
				return nil, fmt.Errorf("load admin memberships: %w", err)
			}
			defer rows.Close()
			for rows.Next() {
				var id goauth.SubjectID
				var adminID int64
				if err := rows.Scan(&adminID, &id); err != nil {
					return nil, err
				}
				result.Members = append(result.Members, id)
				result.AdminIDs[adminID] = id
			}
			return result.Members, rows.Err()
		})
		if err != nil {
			return authsnapshot.Data{}, err
		}
		result.RBAC = snapshot
		return result, nil
	}
}

// ConfigureMembershipMutation is an internal startup-only contract implemented
// by the GoAdmin repository. It wraps every membership-changing SQL operation.
type membershipMutationConfigurer interface {
	ConfigureMembershipMutation(fn func(context.Context, func(context.Context) error) error)
}

func (a *Adapter) configureMembershipInvalidation(target membershipMutationConfigurer) {
	observer := &postgres.AuthTransactionObserver{Notify: func(outcome postgres.AuthTransactionOutcome) {
		if outcome == postgres.AuthTransactionCommitted || outcome == postgres.AuthTransactionUnknown {
			// Invalidation does no I/O; cancelled HTTP requests must not preserve grants.
			a.authorization.NotifyChange()
		}
	}}
	target.ConfigureMembershipMutation(func(ctx context.Context, fn func(context.Context) error) error {
		if pgstorage.GetTx(ctx) != nil {
			return errors.New("snapshot membership writes require canonical auth transaction, not ambient pgx transaction")
		}
		return a.runtime.InAuthTransaction(ctx, func(txCtx context.Context) error {
			if err := a.runtime.ObserveAuthTransaction(txCtx, observer); err != nil {
				return err
			}
			executor, err := a.runtime.SQLExecutor(txCtx)
			if err != nil {
				return err
			}
			return fn(admintx.WithExecutor(txCtx, executor, a.database.DB().Pool()))
		})
	})
}

// The Runtime may also serve user-realm RBAC consumers. Register invalidation
// support without feeding them admin-only authorization answers. Admin guards
// read the snapshot directly; generic GoAuth RBAC retains canonical semantics.
type authorizationInvalidator struct{ cache *authsnapshot.Cache }

func (authorizationInvalidator) HasPermission(
	context.Context, goauth.SubjectID, rbac.PermissionKey,
) (allowed bool, found bool, err error) {
	return false, false, nil
}

func (c authorizationInvalidator) Invalidate(ctx context.Context) error {
	return c.cache.Invalidate(ctx)
}
