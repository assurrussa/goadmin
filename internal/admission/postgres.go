package admission

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/postgres"
	"github.com/assurrussa/goauth/rbac"
	"github.com/google/uuid"
)

const ownerRoleSlug = "super_admin"

type strictProvisioner interface {
	ProvisionExternalIdentity(ctx context.Context, identity goauth.ExternalIdentity) (goauth.Account, error)
}
type Postgres struct {
	runtime     *postgres.Runtime
	provisioner strictProvisioner
	roles       *rbac.Service
	now         func() time.Time
}
type pgTx struct {
	store *Postgres
	sql   postgres.SQLExecutor
}

func NewPostgres(runtime *postgres.Runtime, now func() time.Time) (*Postgres, error) {
	if runtime == nil || runtime.Runtime == nil {
		return nil, ErrInvalid
	}
	provisioner, ok := any(runtime.Runtime).(strictProvisioner)
	if !ok {
		return nil, errors.New("admission requires strict canonical SSO provisioning support")
	}
	roles, err := runtime.RBAC(nil)
	if err != nil {
		return nil, err
	}
	if now == nil {
		now = time.Now
	}
	return &Postgres{runtime: runtime, provisioner: provisioner, roles: roles, now: now}, nil
}

func (s *Postgres) Owned(ctx context.Context, fn func(context.Context, Tx) error) error {
	return s.runtime.InOwnedAuthTransaction(ctx, func(ctx context.Context) error {
		db, err := s.runtime.SQLExecutor(ctx)
		if err != nil {
			return err
		}
		return fn(ctx, &pgTx{store: s, sql: db})
	})
}

func (t *pgTx) LockIdentity(ctx context.Context, scope Scope, subject string) error {
	key, err := json.Marshal([]string{"goadmin-admission", scope.Issuer, scope.Project, scope.Client, subject})
	if err != nil {
		return err
	}
	_, err = t.sql.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", string(key))
	return err
}

const recordColumns = "id::text,issuer,project_id,client_id,external_subject,subject_id::text,status,version," +
	"COALESCE(approved_by::text,''),grant_action,roles,expires_at"

type scanner interface{ Scan(dest ...any) error }

func scanRecord(row scanner) (Record, error) {
	var r Record
	var raw []byte
	var until sql.NullTime
	err := row.Scan(&r.ID, &r.Scope.Issuer, &r.Scope.Project, &r.Scope.Client, &r.ExternalSubject, &r.LocalSubject,
		&r.Status, &r.Version, &r.ApprovedBy, &r.GrantAction, &raw, &until)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, err
	}
	if err = json.Unmarshal(raw, &r.Roles); err != nil {
		return Record{}, err
	}
	if until.Valid {
		r.ExpiresAt = until.Time
	}
	return r, nil
}

func (t *pgTx) Find(ctx context.Context, scope Scope, subject string) (Record, error) {
	return scanRecord(t.sql.QueryRowContext(ctx, "SELECT "+recordColumns+`
 FROM admin_admissions WHERE issuer=$1 AND project_id=$2 AND client_id=$3 AND external_subject=$4 FOR UPDATE`,
		scope.Issuer, scope.Project, scope.Client, subject))
}

func nullableTime(v time.Time) any {
	if v.IsZero() {
		return nil
	}
	return v.UTC()
}

func (t *pgTx) Save(ctx context.Context, r Record, expected int64) (Record, error) {
	raw, err := json.Marshal(r.Roles)
	if err != nil {
		return Record{}, err
	}
	if r.Roles == nil {
		raw = []byte("[]")
	}
	if expected == 0 {
		return scanRecord(t.sql.QueryRowContext(ctx, `INSERT INTO admin_admissions
 (id,issuer,project_id,client_id,external_subject,subject_id,status,version,approved_by,grant_action,roles,expires_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,1,NULLIF($8,'')::uuid,$9,$10,$11) RETURNING `+recordColumns,
			r.ID, r.Scope.Issuer, r.Scope.Project, r.Scope.Client, r.ExternalSubject, r.LocalSubject, r.Status,
			r.ApprovedBy, r.GrantAction, raw, nullableTime(r.ExpiresAt)))
	}
	updated, err := scanRecord(t.sql.QueryRowContext(ctx, `UPDATE admin_admissions SET
 status=$1,version=version+1,approved_by=NULLIF($2,'')::uuid,grant_action=$3,roles=$4,expires_at=$5,updated_at=now()
 WHERE id=$6 AND version=$7 AND subject_id=$8 RETURNING `+recordColumns,
		r.Status, r.ApprovedBy, r.GrantAction, raw, nullableTime(r.ExpiresAt), r.ID, expected, r.LocalSubject))
	if errors.Is(err, ErrNotFound) {
		return Record{}, ErrConflict
	}
	return updated, err
}

func (t *pgTx) Invitation(ctx context.Context, id string) (Invitation, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Invitation{}, ErrInvalid
	}
	var i Invitation
	var raw []byte
	err := t.sql.QueryRowContext(ctx, `SELECT id::text,issuer,project_id,client_id,external_subject,
 issued_by::text,roles,expires_at,revoked,COALESCE(accepted_subject::text,'')
 FROM admin_admission_invitations WHERE id=$1 FOR UPDATE`, id).Scan(
		&i.ID, &i.Scope.Issuer, &i.Scope.Project, &i.Scope.Client, &i.ExternalSubject,
		&i.IssuedBy, &raw, &i.ExpiresAt, &i.Revoked, &i.AcceptedSubject)
	if errors.Is(err, sql.ErrNoRows) {
		return Invitation{}, ErrNotFound
	}
	if err != nil {
		return Invitation{}, err
	}
	err = json.Unmarshal(raw, &i.Roles)
	return i, err
}

func (t *pgTx) CreateInvitation(ctx context.Context, i Invitation) error {
	raw, err := json.Marshal(i.Roles)
	if err != nil {
		return err
	}
	_, err = t.sql.ExecContext(ctx, `INSERT INTO admin_admission_invitations
 (id,issuer,project_id,client_id,external_subject,issued_by,roles,expires_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, i.ID, i.Scope.Issuer, i.Scope.Project, i.Scope.Client,
		i.ExternalSubject, i.IssuedBy, raw, i.ExpiresAt)
	return err
}

func oneRow(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConflict
	}
	return nil
}

func (t *pgTx) ConsumeInvitation(ctx context.Context, id, subject string) error {
	return oneRow(t.sql.ExecContext(ctx, `UPDATE admin_admission_invitations SET accepted_subject=$1
 WHERE id=$2 AND NOT revoked AND accepted_subject IS NULL AND expires_at>$3`, subject, id, t.store.now()))
}

func (t *pgTx) RevokeInvitation(ctx context.Context, id string) error {
	return oneRow(t.sql.ExecContext(ctx, `UPDATE admin_admission_invitations SET revoked=true
 WHERE id=$1 AND accepted_subject IS NULL`, id))
}

// GrantPermission is a concrete role grant, independent of roles held by actor.
// Permission keys are bound to a role's immutable public ID, not its reusable name.
func GrantPermission(publicID string) (rbac.PermissionKey, error) {
	id, err := uuid.Parse(publicID)
	if err != nil {
		return "", ErrInvalid
	}
	return rbac.NewPermissionKey("admission", "grant-"+id.String())
}

func (t *pgTx) lockActiveSubject(ctx context.Context, id string) error {
	if _, err := goauth.ParseSubjectID(id); err != nil {
		return ErrDenied
	}
	var status string
	err := t.sql.QueryRowContext(ctx, "SELECT status FROM auth_subjects WHERE id=$1 FOR UPDATE", id).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrDenied
	}
	if err != nil {
		return err
	}
	if status != "active" {
		return ErrDenied
	}
	return nil
}

func (t *pgTx) CheckActive(ctx context.Context, id string) error {
	if err := t.lockActiveSubject(ctx, id); err != nil {
		return err
	}
	var admin int64
	err := t.sql.QueryRowContext(ctx,
		"SELECT id FROM administrations WHERE subject_id=$1 AND deleted_at IS NULL FOR UPDATE", id).Scan(&admin)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrDenied
	}
	return err
}

// Keep a deterministic order for overlapping role sets. Do not hold a table
// lock while waiting for row locks acquired by the canonical RBAC editor.
func (t *pgTx) lockGrantRoles(ctx context.Context, actor string, ids []int64) error {
	rows, err := t.sql.QueryContext(ctx, `SELECT r.id FROM auth_roles r
 WHERE r.id IN (SELECT role_id FROM auth_subject_roles WHERE subject_id=$1)
 OR r.id=ANY($2::bigint[]) ORDER BY r.id FOR SHARE OF r`, actor, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return rows.Close()
}

func (t *pgTx) Authorize(ctx context.Context, actor, action string, ids []int64) ([]Role, error) {
	switch action {
	case "read", actionInvite, actionApprove, "reject":
	default:
		return nil, ErrDenied
	}
	if err := t.CheckActive(ctx, actor); err != nil {
		return nil, err
	}
	// Pin grantor and requested role definitions before reading permission sets.
	// Canonical role editors take FOR UPDATE before changing roles/permissions;
	// FOR SHARE blocks those edits while remaining compatible with activation's
	// foreign-key checks. CheckActive's subject lock also fences canonical role
	// assignment/replacement for this grantor. Unrelated role edits stay free.
	if err := t.lockGrantRoles(ctx, actor, ids); err != nil {
		return nil, err
	}
	subject, err := goauth.ParseSubjectID(actor)
	if err != nil {
		return nil, err
	}
	key, err := rbac.NewPermissionKey("admission", action)
	if err != nil {
		return nil, err
	}
	ok, err := t.store.roles.Check(ctx, subject, key)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrDenied
	}
	if len(ids) > 0 {
		assign, err := rbac.NewPermissionKey("roles", "assign")
		if err != nil {
			return nil, err
		}
		ok, err = t.store.roles.Check(ctx, subject, assign)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrDenied
		}
	}
	result := make([]Role, 0, len(ids))
	for _, id := range ids {
		role, err := t.store.roles.Role(ctx, id)
		if err != nil {
			return nil, err
		}
		// Owner/bootstrap identities are never provisioned by this component.
		if role.Slug == ownerRoleSlug {
			return nil, ErrDenied
		}
		grant, err := GrantPermission(role.PublicID)
		if err != nil {
			return nil, err
		}
		ok, err := t.store.roles.Check(ctx, subject, grant)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrDenied
		}
		permissions, err := t.store.roles.RolePermissions(ctx, id)
		if err != nil {
			return nil, err
		}
		keys := make([]string, 0, len(permissions))
		for _, p := range permissions {
			keys = append(keys, string(p.Key))
		}
		slices.Sort(keys)
		digest := sha256.Sum256([]byte(role.PublicID + "\n" + strings.Join(keys, "\n")))
		result = append(result, Role{ID: id, PublicID: role.PublicID, Name: role.Name, Fingerprint: hex.EncodeToString(digest[:])})
	}
	return result, nil
}

func (t *pgTx) Provision(ctx context.Context, i Identity) (string, error) {
	account, err := t.store.provisioner.ProvisionExternalIdentity(ctx, goauth.ExternalIdentity{
		Issuer: i.Scope.Issuer, Subject: i.Subject, Email: i.Email, EmailVerified: i.EmailVerified,
		Profile: goauth.BasicProfile{DisplayName: i.DisplayName},
	})
	if err != nil {
		return "", err
	}
	id := account.Subject.ID.String()
	if err := t.lockActiveSubject(ctx, id); err != nil {
		return "", err
	}
	if !account.EmailVerified() {
		return "", ErrDenied
	}
	var deleted sql.NullTime
	err = t.sql.QueryRowContext(ctx, "SELECT deleted_at FROM administrations WHERE subject_id=$1 FOR UPDATE", id).Scan(&deleted)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if err == nil && deleted.Valid {
		return "", ErrDenied
	}
	return id, nil
}

func (t *pgTx) Activate(ctx context.Context, id string, roles []Role) error {
	if err := t.lockActiveSubject(ctx, id); err != nil {
		return err
	}
	// Never restore or rewrite a pre-existing membership via enrollment.
	result, err := t.sql.ExecContext(ctx, `INSERT INTO administrations(subject_id,uuid,version,data)
 VALUES($1,$2,0,'{}'::jsonb) ON CONFLICT(subject_id) DO NOTHING`, id, uuid.NewString())
	if err := oneRow(result, err); err != nil {
		return err
	}
	subject, err := goauth.ParseSubjectID(id)
	if err != nil {
		return err
	}
	return t.store.roles.ReplaceSubjectRoles(ctx, subject, roleIDs(roles))
}

func (t *pgTx) Audit(ctx context.Context, actor, action, target string) error {
	_, err := t.sql.ExecContext(ctx, `INSERT INTO admin_admission_events(actor_subject_id,action,target_id)
 VALUES($1,$2,$3)`, actor, action, target)
	if err != nil {
		return err
	}
	// Reuse the established admin audit for actions by an admitted operator.
	_, err = t.sql.ExecContext(ctx, `INSERT INTO admin_action_audit
 (actor_admin_id,actor_subject_id,action,target_type,target_id)
 SELECT id,subject_id::text,$2,'admission',$3 FROM administrations
 WHERE subject_id=$1 AND deleted_at IS NULL`, actor, action, target)
	return err
}

func (s *Postgres) List(ctx context.Context, actor string, scope Scope) ([]Record, error) {
	var result []Record
	err := s.Owned(ctx, func(ctx context.Context, tx Tx) error {
		if _, err := tx.Authorize(ctx, actor, "read", nil); err != nil {
			return err
		}
		pg, ok := tx.(*pgTx)
		if !ok {
			return ErrInvalid
		}
		db := pg.sql
		rows, err := db.QueryContext(ctx, "SELECT "+recordColumns+` FROM admin_admissions
 WHERE issuer=$1 AND project_id=$2 AND client_id=$3 ORDER BY created_at DESC,id DESC LIMIT 100`,
			scope.Issuer, scope.Project, scope.Client)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			r, err := scanRecord(rows)
			if err != nil {
				return err
			}
			result = append(result, r)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Postgres) GrantableRoles(ctx context.Context, actor, action string) ([]Role, error) {
	var result []Role
	err := s.Owned(ctx, func(ctx context.Context, tx Tx) error {
		if _, err := tx.Authorize(ctx, actor, "read", nil); err != nil {
			return err
		}
		all, err := s.roles.Roles(ctx, rbac.RoleFilter{Limit: 1000})
		if err != nil {
			return err
		}
		for _, role := range all {
			if role.Slug == ownerRoleSlug {
				continue
			}
			grants, err := tx.Authorize(ctx, actor, action, []int64{role.ID})
			if errors.Is(err, ErrDenied) {
				continue
			}
			if err != nil {
				return err
			}
			result = append(result, grants...)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (t *pgTx) CheckIdentity(ctx context.Context, i Identity, id string) error {
	if err := t.lockActiveSubject(ctx, id); err != nil {
		return err
	}
	var linked string
	err := t.sql.QueryRowContext(ctx, `SELECT subject_id::text FROM auth_identity_links
 WHERE issuer=$1 AND external_subject=$2 FOR SHARE`, i.Scope.Issuer, i.Subject).Scan(&linked)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrDenied
	}
	if err != nil {
		return err
	}
	if linked != id {
		return ErrDenied
	}
	return nil
}

func (s *Postgres) Permissions(ctx context.Context) ([]rbac.Permission, error) {
	result := []rbac.Permission{}
	for _, action := range []string{"read", actionInvite, actionApprove, "reject"} {
		key, err := rbac.NewPermissionKey("admission", action)
		if err != nil {
			return nil, err
		}
		result = append(result, rbac.Permission{Key: key, Description: "SSO admission: " + action})
	}
	roles, err := s.roles.Roles(ctx, rbac.RoleFilter{Limit: 1000})
	if err != nil {
		return nil, err
	}
	for _, role := range roles {
		if role.Slug == ownerRoleSlug {
			continue
		}
		key, err := GrantPermission(role.PublicID)
		if err != nil {
			return nil, err
		}
		result = append(result, rbac.Permission{Key: key, Description: "Grant role: " + role.Name})
	}
	return result, nil
}

func (s *Postgres) Invitations(ctx context.Context, actor string, scope Scope) ([]Invitation, error) {
	var result []Invitation
	err := s.Owned(ctx, func(ctx context.Context, tx Tx) error {
		if _, err := tx.Authorize(ctx, actor, "read", nil); err != nil {
			return err
		}
		pg, ok := tx.(*pgTx)
		if !ok {
			return ErrInvalid
		}
		rows, err := pg.sql.QueryContext(ctx, `SELECT id::text,external_subject,issued_by::text,roles,expires_at,revoked,
 COALESCE(accepted_subject::text,'') FROM admin_admission_invitations
 WHERE issuer=$1 AND project_id=$2 AND client_id=$3 ORDER BY created_at DESC,id DESC LIMIT 100`,
			scope.Issuer, scope.Project, scope.Client)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			i := Invitation{Scope: scope}
			var raw []byte
			if err := rows.Scan(&i.ID, &i.ExternalSubject, &i.IssuedBy, &raw, &i.ExpiresAt, &i.Revoked, &i.AcceptedSubject); err != nil {
				return err
			}
			if err := json.Unmarshal(raw, &i.Roles); err != nil {
				return err
			}
			result = append(result, i)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Postgres) Allowed(ctx context.Context, actor, action string) (bool, error) {
	err := s.Owned(ctx, func(ctx context.Context, tx Tx) error {
		_, err := tx.Authorize(ctx, actor, action, nil)
		return err
	})
	if errors.Is(err, ErrDenied) {
		return false, nil
	}
	return err == nil, err
}
