package adminservice

import (
	"context"
	"errors"
	"time"

	pgstore "github.com/assurrussa/outbox/backends/pgsql/storage"
	"github.com/jackc/pgx/v5"

	"github.com/assurrussa/goadmin/internal/admintx"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
)

// CheckConfirmedAction verifies the original native login and holds its
// effective grants until the transaction that changes the record commits.
func (s *Service) CheckConfirmedAction(
	ctx context.Context, b ReadSessionBinding, permissions []integrationroles.PermissionKey,
) error {
	_, err := s.ConfirmedActionDeadline(ctx, b, permissions)
	return err
}

// ConfirmedActionDeadline checks the confirmed action's native authority and
// returns its earliest known expiry. Recheck after host work to observe expiry
// of the Fiber storage lease as well as the canonical deadline.
func (s *Service) ConfirmedActionDeadline(
	ctx context.Context, b ReadSessionBinding, permissions []integrationroles.PermissionKey,
) (time.Time, error) {
	tx := pgstore.GetTx(ctx)
	if tx == nil || b.external != nil || len(permissions) == 0 {
		return time.Time{}, ErrReadSessionDenied
	}
	ctx = admintx.WithSessionReadTx(ctx, tx)
	var expires time.Time
	var lease delegatedSessionLease
	const login = `SELECT l.expires_at,clock_timestamp() FROM auth_subjects s
		JOIN administrations a ON a.subject_id=s.id
		JOIN auth_sessions l ON l.subject_id=s.id
		WHERE s.id=$1 AND a.id=$2 AND a.deleted_at IS NULL AND s.status='active' AND s.retired_at IS NULL
		AND l.id=$3 AND l.realm='admin' AND l.scope='authenticated' AND l.revoked_at IS NULL
		AND l.security_version=s.security_version FOR SHARE OF s,a,l`
	if err := tx.QueryRow(ctx, login, b.subject.String(), b.adminID, b.loginID).Scan(&expires, &lease.base); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, ErrReadSessionEnded
		}
		return time.Time{}, err
	}
	const allowed = `SELECT 1 FROM auth_subject_roles sr
		JOIN auth_role_permissions rp ON rp.role_id=sr.role_id JOIN auth_permissions p ON p.id=rp.permission_id
		WHERE sr.subject_id=$1 AND p.permission_key=$2 ORDER BY sr.role_id LIMIT 1 FOR SHARE OF sr,rp,p`
	for _, permission := range permissions {
		var present int
		if err := tx.QueryRow(ctx, allowed, b.subject.String(), permission.String()).Scan(&present); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return time.Time{}, ErrReadSessionDenied
			}
			return time.Time{}, err
		}
	}
	sess, err := s.openDelegatedSession(ctx, b, &lease)
	if err != nil {
		return time.Time{}, err
	}
	defer sess.Release()
	if sess.Get(integrationroles.RolePreviewSessionKey) != nil {
		return time.Time{}, ErrReadSessionDenied
	}
	browserExpires, err := s.checkDelegatedBrowser(ctx, b, sess)
	if err != nil {
		return time.Time{}, err
	}
	fiberExpires, err := fiberAbsoluteExpiry(sess, s.store.AbsoluteTimeout)
	if err != nil {
		return time.Time{}, err
	}
	deadline := earliestMaintenanceExpiry(expires, browserExpires, fiberExpires, lease.expires)
	if !time.Now().Before(deadline) {
		return time.Time{}, ErrReadSessionEnded
	}
	return deadline, nil
}
