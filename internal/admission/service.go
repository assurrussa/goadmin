// Package admission implements the optional enrollment policy without issuing
// browser sessions. Persistence and canonical authority are supplied by the
// GoAdmin-owned adapter, never by request JSON.
package admission

import (
	"context"
	"errors"
	"slices"
	"time"
)

type Mode string
const (
	InvitationOnly Mode = "invitation"
	ProjectPending Mode = "project-pending"
)
type Status string
const (
	Pending Status = "pending"
	Approved Status = "approved"
	Active Status = "active"
	Rejected Status = "rejected"
)
var (
	ErrDenied = errors.New("admission denied")
	ErrConflict = errors.New("admission state changed")
	ErrInvalid = errors.New("invalid admission request")
	ErrNotFound = errors.New("admission record not found")
)

type Scope struct { Issuer, Project, Client string }
type Identity struct {
	Scope Scope
	Subject, Email, DisplayName string
	EmailVerified bool
	// ExpiresAt is bounded by the original verified proof and provider session.
	ExpiresAt time.Time
}
type Role struct {
	ID int64
	PublicID string
	// Fingerprint pins the exact role permission set at authorization time.
	Fingerprint string
}
type Record struct {
	ID string
	Scope Scope
	ExternalSubject, LocalSubject string
	Status Status
	Version int64
	ApprovedBy, GrantAction string
	Roles []Role
	ExpiresAt time.Time
}
type Invitation struct {
	ID string
	Scope Scope
	ExternalSubject, IssuedBy string
	Roles []Role
	ExpiresAt time.Time
	Revoked bool
	AcceptedSubject string
}

// Verifier accepts an opaque host/RP reference, never user-decoded claims.
// Verify may contact the provider before the transaction. Revalidate must be a
// local, read-only authority check; it must not refresh or perform network I/O.
type Verifier interface {
	Verify(context.Context, string) (Identity, error)
	Revalidate(context.Context, string, Identity) error
}
type Store interface {
	// Owned commits once and returns nil only for confirmed commit. Callback
	// errors roll back; unknown outcomes must stay errors. No automatic retry.
	Owned(context.Context, func(context.Context, Tx) error) error
}
type Tx interface {
	// LockIdentity serializes exact scope/issuer/subject enrollment, including
	// the missing-row case, without changing global subject status.
	LockIdentity(context.Context, Scope, string) error
	Find(context.Context, Scope, string) (Record, error)
	Save(context.Context, Record, int64) (Record, error)
	Invitation(context.Context, string) (Invitation, error)
	CreateInvitation(context.Context, Invitation) error
	ConsumeInvitation(context.Context, string, string) error
	RevokeInvitation(context.Context, string) error
	// Authorize checks live canonical actor + admin membership + explicit
	// admission management and role-grant permissions. It locks the relevant
	// authority through commit. No super-role or own-role hierarchy shortcut.
	Authorize(context.Context, string, string, []int64) ([]Role, error)
	// Provision uses exact identity and strict no-email-autolink canonical API.
	// Existing disabled/retired accounts and deleted memberships must deny.
	Provision(context.Context, Identity) (string, error)
	// Activate atomically creates the local membership and assigns roles using
	// the canonical Runtime/RBAC transaction, without a local credential.
	Activate(context.Context, string, []Role) error
	// CheckActive denies revoked membership and disabled/retired subjects.
	CheckActive(context.Context, string) error
	Audit(context.Context, string, string, string) error
}

type Config struct {
	Mode Mode
	Scope Scope
	Now func() time.Time
	NewID func() string
	MaxInvitationTTL time.Duration
}
type Service struct { cfg Config; store Store; verifier Verifier }

func New(cfg Config, store Store, verifier Verifier) (*Service, error) {
	if (cfg.Mode != InvitationOnly && cfg.Mode != ProjectPending) ||
		cfg.Scope.Issuer == "" || cfg.Scope.Project == "" || cfg.Scope.Client == "" ||
		cfg.NewID == nil || store == nil || verifier == nil {
		return nil, ErrInvalid
	}
	if cfg.Now == nil { cfg.Now = time.Now }
	if cfg.MaxInvitationTTL == 0 { cfg.MaxInvitationTTL = 7 * 24 * time.Hour }
	if cfg.MaxInvitationTTL <= 0 || cfg.MaxInvitationTTL > 30 * 24 * time.Hour { return nil, ErrInvalid }
	return &Service{cfg: cfg, store: store, verifier: verifier}, nil
}
func (s *Service) validIdentity(i Identity) bool {
	return i.Scope == s.cfg.Scope && i.Subject != "" && len(i.Subject) <= 512 &&
		i.EmailVerified && i.Email != "" && s.cfg.Now().Before(i.ExpiresAt)
}
func sameRoles(a, b []Role) bool {
	if len(a) != len(b) { return false }
	for n := range a { if a[n] != b[n] { return false } }
	return true
}
func roleIDs(roles []Role) []int64 {
	ids := make([]int64, len(roles))
	for i := range roles { ids[i] = roles[i].ID }
	return ids
}
func normalizedIDs(ids []int64) ([]int64, error) {
	if len(ids) == 0 || len(ids) > 32 { return nil, ErrInvalid }
	result := slices.Clone(ids)
	slices.Sort(result)
	for _, id := range result { if id <= 0 { return nil, ErrInvalid } }
	return slices.Compact(result), nil
}

// Admit is called only after a successful SSO callback. It never returns native
// session credentials; an Active result must still traverse LoginExternalAdmin.
func (s *Service) Admit(ctx context.Context, reference, invitationID string) (Record, error) {
	identity, err := s.verifier.Verify(ctx, reference)
	if err != nil { return Record{}, err }
	if !s.validIdentity(identity) { return Record{}, ErrDenied }
	var result Record
	err = s.store.Owned(ctx, func(txCtx context.Context, tx Tx) error {
		if err := tx.LockIdentity(txCtx, identity.Scope, identity.Subject); err != nil { return err }
		if !s.validIdentity(identity) { return ErrDenied }
		if err := s.verifier.Revalidate(txCtx, reference, identity); err != nil { return err }
		current, err := tx.Find(txCtx, identity.Scope, identity.Subject)
		if err != nil && !errors.Is(err, ErrNotFound) { return err }
		if err == nil {
			local, checkErr := tx.Provision(txCtx, identity)
			if checkErr != nil { return checkErr }
			if local != current.LocalSubject { return ErrConflict }
			switch current.Status {
			case Rejected:
				return ErrDenied
			case Active:
				if err := tx.CheckActive(txCtx, current.LocalSubject); err != nil { return err }
				result = current
				return nil
			case Approved:
				return s.activate(txCtx, tx, current, identity, &result)
			case Pending:
				if invitationID == "" { result = current; return nil }
			default:
				return ErrDenied
			}
		}
		var invite Invitation
		if invitationID != "" {
			invite, err = tx.Invitation(txCtx, invitationID)
			if err != nil { return err }
			if invite.Scope != identity.Scope || invite.ExternalSubject != identity.Subject ||
				invite.Revoked || invite.AcceptedSubject != "" || !s.cfg.Now().Before(invite.ExpiresAt) {
				return ErrDenied
			}
			roles, authErr := tx.Authorize(txCtx, invite.IssuedBy, "invite", roleIDs(invite.Roles))
			if authErr != nil { return authErr }
			if !sameRoles(roles, invite.Roles) { return ErrConflict }
		} else if s.cfg.Mode == InvitationOnly {
			return ErrDenied
		}
		local, err := tx.Provision(txCtx, identity)
		if err != nil { return err }
		expected := current.Version
		if current.ID == "" {
			current = Record{ID:s.cfg.NewID(), Scope:identity.Scope,
				ExternalSubject:identity.Subject, LocalSubject:local, Status:Pending}
		} else if current.LocalSubject != local { return ErrConflict }
		if invite.ID != "" {
			current.Status = Approved
			current.ApprovedBy = invite.IssuedBy
			current.GrantAction = "invite"
			current.Roles = slices.Clone(invite.Roles)
			current.ExpiresAt = invite.ExpiresAt
		}
		current, err = tx.Save(txCtx, current, expected)
		if err != nil { return err }
		if err := tx.Audit(txCtx, local, "admission.requested", current.ID); err != nil { return err }
		if invite.ID != "" {
			if err := s.activate(txCtx, tx, current, identity, &result); err != nil { return err }
			return tx.ConsumeInvitation(txCtx, invite.ID, local)
		}
		result = current
		return nil
	})
	if err != nil { return Record{}, err }
	return result, nil
}

func (s *Service) activate(ctx context.Context, tx Tx, record Record, identity Identity, result *Record) error {
	if !s.validIdentity(identity) || !s.cfg.Now().Before(record.ExpiresAt) { return ErrDenied }
	if record.GrantAction != "approve" && record.GrantAction != "invite" { return ErrDenied }
	roles, err := tx.Authorize(ctx, record.ApprovedBy, record.GrantAction, roleIDs(record.Roles))
	if err != nil { return err }
	if !sameRoles(roles, record.Roles) { return ErrConflict }
	local, err := tx.Provision(ctx, identity)
	if err != nil { return err }
	if local != record.LocalSubject { return ErrConflict }
	if err := tx.Activate(ctx, local, roles); err != nil { return err }
	record.Status = Active
	updated, err := tx.Save(ctx, record, record.Version)
	if err != nil { return err }
	if err := tx.Audit(ctx, record.ApprovedBy, "admission.activated", record.ID); err != nil { return err }
	*result = updated
	return nil
}

func (s *Service) Invite(ctx context.Context, actor, subject string, ids []int64, until time.Time) (Invitation, error) {
	ids, err := normalizedIDs(ids)
	if err != nil || subject == "" || len(subject) > 512 ||
		!until.After(s.cfg.Now()) || until.Sub(s.cfg.Now()) > s.cfg.MaxInvitationTTL { return Invitation{}, ErrInvalid }
	invite := Invitation{ID:s.cfg.NewID(), Scope:s.cfg.Scope, ExternalSubject:subject,
		IssuedBy:actor, ExpiresAt:until}
	err = s.store.Owned(ctx, func(ctx context.Context, tx Tx) error {
		if err := tx.LockIdentity(ctx, s.cfg.Scope, subject); err != nil { return err }
		roles, err := tx.Authorize(ctx, actor, "invite", ids)
		if err != nil { return err }
		invite.Roles = roles
		if !s.cfg.Now().Before(until) { return ErrDenied }
		if err := tx.CreateInvitation(ctx, invite); err != nil { return err }
		return tx.Audit(ctx, actor, "admission.invited", invite.ID)
	})
	if err != nil { return Invitation{}, err }
	return invite, nil
}

// Approve records a bounded local decision. Actual activation waits for another
// successful, fresh SSO admission, so an offline central grant cannot be bypassed.
func (s *Service) Approve(ctx context.Context, actor, subject string, version int64, ids []int64, until time.Time) error {
	ids, err := normalizedIDs(ids)
	if err != nil || version < 1 || !until.After(s.cfg.Now()) ||
		until.Sub(s.cfg.Now()) > s.cfg.MaxInvitationTTL { return ErrInvalid }
	return s.store.Owned(ctx, func(ctx context.Context, tx Tx) error {
		if err := tx.LockIdentity(ctx, s.cfg.Scope, subject); err != nil { return err }
		current, err := tx.Find(ctx, s.cfg.Scope, subject)
		if err != nil { return err }
		if current.Version != version || current.Status != Pending { return ErrConflict }
		roles, err := tx.Authorize(ctx, actor, "approve", ids)
		if err != nil { return err }
		if !s.cfg.Now().Before(until) { return ErrDenied }
		current.Status, current.ApprovedBy, current.Roles, current.ExpiresAt = Approved, actor, roles, until
		current.GrantAction = "approve"
		if _, err := tx.Save(ctx, current, version); err != nil { return err }
		return tx.Audit(ctx, actor, "admission.approved", current.ID)
	})
}
func (s *Service) Reject(ctx context.Context, actor, subject string, version int64) error {
	return s.store.Owned(ctx, func(ctx context.Context, tx Tx) error {
		if err := tx.LockIdentity(ctx, s.cfg.Scope, subject); err != nil { return err }
		current, err := tx.Find(ctx, s.cfg.Scope, subject)
		if err != nil { return err }
		if current.Version != version || (current.Status != Pending && current.Status != Approved) { return ErrConflict }
		if _, err := tx.Authorize(ctx, actor, "reject", nil); err != nil { return err }
		current.Status = Rejected
		if _, err := tx.Save(ctx, current, version); err != nil { return err }
		return tx.Audit(ctx, actor, "admission.rejected", current.ID)
	})
}
func (s *Service) Revoke(ctx context.Context, actor, id string) error {
	return s.store.Owned(ctx, func(ctx context.Context, tx Tx) error {
		if _, err := tx.Authorize(ctx, actor, "invite", nil); err != nil { return err }
		invite, err := tx.Invitation(ctx, id)
		if err != nil { return err }
		if invite.Scope != s.cfg.Scope || invite.AcceptedSubject != "" { return ErrDenied }
		if err := tx.RevokeInvitation(ctx, id); err != nil { return err }
		return tx.Audit(ctx, actor, "admission.invitation-revoked", id)
	})
}
