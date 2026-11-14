package host

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/assurrussa/goauth"
)

// ErrAdminMembershipUnavailable means the subject is not an active administrator.
// It does not distinguish missing, suspended, retired or revoked identities.
var ErrAdminMembershipUnavailable = errors.New("active admin membership unavailable")

// RequireActiveAdmin checks canonical identity status and current admin membership.
// It grants no permissions or credentials. The query joins the adapter's managed
// auth transaction in ctx and locks identity and membership rows until it ends.
// Call inside Runtime().InAuthTransaction (or an auth policy callback) when a
// subsequent mutation must be fenced against concurrent suspension or revocation.
// Outside a transaction this is only a point-in-time check. Contexts belonging to
// another auth database handle are rejected by the canonical Runtime.
func (a *AuthAdapter) RequireActiveAdmin(ctx context.Context, subject goauth.SubjectID) error {
	if ctx == nil || !a.valid() || subject.IsZero() {
		return ErrAdminMembershipUnavailable
	}
	executor, err := a.Runtime().SQLExecutor(ctx)
	if err != nil {
		return err
	}
	var found string
	err = executor.QueryRowContext(ctx, `SELECT a.subject_id::text FROM auth_subjects s
 JOIN administrations a ON a.subject_id=s.id
 WHERE s.id=$1 AND s.status='active' AND s.retired_at IS NULL AND a.deleted_at IS NULL
 FOR SHARE OF s,a`, subject).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrAdminMembershipUnavailable
	}
	if err != nil {
		return fmt.Errorf("check active admin membership: %w", err)
	}
	return nil
}
