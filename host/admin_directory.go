package host

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/assurrussa/goauth"
)

var (
	ErrAdminDirectoryDenied      = errors.New("admin directory access denied")
	ErrAdminDirectoryInvalid     = errors.New("invalid admin directory request")
	ErrAdminDirectoryUnavailable = errors.New("active administrator not found")
)

// AdminDirectoryAuthorizer checks current application permission, not session claims.
type AdminDirectoryAuthorizer interface {
	CheckPermission(ctx context.Context, subjectID string, key PermissionKey) error
}

// AdminDirectoryEntry is a minimal display projection. SubjectID is the canonical
// identity to persist in host-owned relations; a site's numeric user ID is not it.
type AdminDirectoryEntry struct {
	SubjectID string `json:"subjectId"`
	Name      string `json:"name"`
	Username  string `json:"username"`
	Email     string `json:"email"`
}

type AdminDirectoryResult struct {
	Entries []AdminDirectoryEntry `json:"entries"`
	More    bool                  `json:"more"`
}

// AdminDirectory borrows the existing canonical database and never closes it.
// It grants no memberships and does not expose routes or enable access screens.
type AdminDirectory struct {
	db          *sql.DB
	permissions AdminDirectoryAuthorizer
	permission  PermissionKey
}

func NewAdminDirectory(db *sql.DB, permissions AdminDirectoryAuthorizer, permission PermissionKey) (*AdminDirectory, error) {
	if db == nil || permissions == nil || permission.IsZero() {
		return nil, ErrAdminDirectoryInvalid
	}
	return &AdminDirectory{db: db, permissions: permissions, permission: permission}, nil
}

func (d *AdminDirectory) authorize(ctx context.Context, actor Actor) error {
	if d == nil || ctx == nil || actor.IsZero() || d.permissions == nil || d.permission.IsZero() {
		return ErrAdminDirectoryDenied
	}
	if _, err := goauth.ParseSubjectID(actor.SubjectID); err != nil {
		return ErrAdminDirectoryDenied
	}
	if d.permissions.CheckPermission(ctx, actor.SubjectID, d.permission) != nil {
		return ErrAdminDirectoryDenied
	}
	return nil
}

// Search returns at most limit active administrators, with literal prefix matching
// on canonical email, username and name. More asks the UI to refine the query.
func (d *AdminDirectory) Search(ctx context.Context, actor Actor, query string, limit int) (AdminDirectoryResult, error) {
	result := AdminDirectoryResult{Entries: []AdminDirectoryEntry{}}
	if err := d.authorize(ctx, actor); err != nil {
		return result, err
	}
	query = strings.TrimSpace(query)
	if !utf8.ValidString(query) || utf8.RuneCountInString(query) < 2 || utf8.RuneCountInString(query) > 128 ||
		strings.ContainsAny(query, "\x00\r\n") || limit < 1 || limit > 50 {
		return result, ErrAdminDirectoryInvalid
	}
	pattern := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(query) + "%"
	const searchSQL = `SELECT a.subject_id::text,
 COALESCE(NULLIF(bp.display_name,''),trim(concat_ws(' ',bp.given_name,bp.family_name)),''),
 COALESCE(bp.username,''),COALESCE(i.display_value,'')
 FROM administrations a
 JOIN auth_subjects s ON s.id=a.subject_id
 LEFT JOIN auth_basic_profiles bp ON bp.subject_id=s.id
 LEFT JOIN auth_identifiers i ON i.subject_id=s.id AND i.scheme='email' AND i.is_primary
 WHERE a.deleted_at IS NULL AND s.status='active' AND s.retired_at IS NULL
 AND (i.display_value ILIKE $1 OR bp.username ILIKE $1 OR bp.given_name ILIKE $1
 OR bp.family_name ILIKE $1 OR bp.display_name ILIKE $1
 OR COALESCE(NULLIF(bp.display_name,''),trim(concat_ws(' ',bp.given_name,bp.family_name)),'') ILIKE $1)
 ORDER BY lower(COALESCE(i.display_value,'')),a.subject_id LIMIT $2`
	rows, err := d.db.QueryContext(ctx, searchSQL, pattern, limit+1)
	if err != nil {
		return result, fmt.Errorf("search admin directory: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var entry AdminDirectoryEntry
		if err = rows.Scan(&entry.SubjectID, &entry.Name, &entry.Username, &entry.Email); err != nil {
			return result, err
		}
		result.Entries = append(result.Entries, entry)
	}
	if err = rows.Err(); err != nil {
		return result, err
	}
	if len(result.Entries) > limit {
		result.More = true
		result.Entries = result.Entries[:limit]
	}
	return result, nil
}

// LockActive validates the chosen subject and holds shared canonical account and
// membership locks until the caller's transaction ends. Call before granting a
// host-owned relation. The caller must authorize the action before BeginTx;
// this primitive checks only the target and never obtains another pooled connection.
// The caller owns commit/rollback. Revocation must remain
// possible without requiring the target account to be active.
func (d *AdminDirectory) LockActive(ctx context.Context, tx *sql.Tx, subject string) error {
	if d == nil || ctx == nil {
		return ErrAdminDirectoryInvalid
	}
	id, err := goauth.ParseSubjectID(subject)
	if err != nil || id.String() != subject || tx == nil {
		return ErrAdminDirectoryInvalid
	}
	const activeSQL = `SELECT a.subject_id::text FROM auth_subjects s
 JOIN administrations a ON a.subject_id=s.id
 WHERE s.id=$1 AND s.status='active' AND s.retired_at IS NULL AND a.deleted_at IS NULL
 FOR SHARE OF s,a`
	var found string
	if err = tx.QueryRowContext(ctx, activeSQL, subject).Scan(&found); errors.Is(err, sql.ErrNoRows) {
		return ErrAdminDirectoryUnavailable
	}
	if err != nil {
		return fmt.Errorf("validate admin directory selection: %w", err)
	}
	return nil
}
