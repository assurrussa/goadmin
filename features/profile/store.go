package profile

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"

	"github.com/assurrussa/goauth/postgres"

	"github.com/assurrussa/goadmin/host"
)

type record struct {
	Version  int64
	Fields   Fields
	Binding  host.ExternalBinding
	Snapshot Snapshot
	Updated  sql.NullTime
	Pending  pending
}

const columns = "version,use_name,use_login,use_avatar,issuer,external_subject,link_id,client_id,project_id," +
	"display_name,display_login,avatar_url,issued_at,updated_at,pending"

func readRecord(ctx context.Context, ex postgres.SQLExecutor, subject string) (record, error) {
	var r record
	var raw []byte
	var issued sql.NullTime
	err := ex.QueryRowContext(ctx, "SELECT "+columns+" FROM admin_profile_presentations WHERE subject_id=$1 FOR UPDATE",
		subject).Scan(&r.Version, &r.Fields.Name, &r.Fields.Login, &r.Fields.Avatar,
		&r.Binding.Issuer, &r.Binding.Subject, &r.Binding.LinkID, &r.Binding.ClientID, &r.Binding.ProjectID,
		&r.Snapshot.Name, &r.Snapshot.Login, &r.Snapshot.AvatarURL, &issued, &r.Updated, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return record{Version: 0}, nil
	}
	if err != nil {
		return record{}, err
	}
	if err = json.Unmarshal(raw, &r.Pending); err != nil {
		return record{}, ErrDenied
	}
	r.Snapshot.IssuedAt = issued.Time
	return r, nil
}

func samePins(a, b host.ExternalBinding) bool {
	return !a.LocalSubject.IsZero() && a.LocalSubject == b.LocalSubject &&
		a.Issuer == b.Issuer && a.Subject == b.Subject && a.LinkID != "" && a.LinkID == b.LinkID &&
		a.ClientID == b.ClientID && a.ProjectID == b.ProjectID
}

func sameProof(a, b host.ExternalBinding) bool {
	return samePins(a, b) && a.AuthorityID == b.AuthorityID && a.LoginGeneration == b.LoginGeneration &&
		a.Generation == b.Generation && a.ProviderSessionID == b.ProviderSessionID &&
		a.Proof.IssuedAt.Equal(b.Proof.IssuedAt) && a.Proof.ExpiresAt.Equal(b.Proof.ExpiresAt) &&
		a.Deadline.Equal(b.Deadline) && a.AbsoluteUntil.Equal(b.AbsoluteUntil) && a.ExternalAuthTime.Equal(b.ExternalAuthTime)
}

func (c *Component) owned(ctx context.Context, binding host.ExternalBinding,
	fn func(context.Context, postgres.SQLExecutor) error,
) error {
	if ctx == nil {
		return ErrDenied
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return c.cfg.Runtime.InOwnedAuthTransaction(bounded, func(ctx context.Context) error {
		ex, err := c.cfg.Runtime.SQLExecutor(ctx)
		if err != nil {
			return err
		}
		var status string
		var retained bool
		if err = ex.QueryRowContext(ctx, "SELECT status,retired_at IS NULL FROM auth_subjects WHERE id=$1 FOR UPDATE",
			binding.LocalSubject).Scan(&status, &retained); err != nil {
			return err
		}
		if status != "active" || !retained {
			return ErrDenied
		}
		var allowed bool
		err = ex.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM administrations WHERE subject_id=$1 AND deleted_at IS NULL)
 AND EXISTS(SELECT 1 FROM auth_identity_links WHERE subject_id=$1 AND id::text=$2 AND issuer=$3 AND external_subject=$4)`,
			binding.LocalSubject, binding.LinkID, binding.Issuer, binding.Subject).Scan(&allowed)
		if err != nil {
			return err
		}
		if !allowed || binding.Issuer != c.cfg.Issuer || binding.ClientID != c.cfg.ClientID ||
			binding.ProjectID != c.cfg.ProjectID {
			return ErrDenied
		}
		return fn(ctx, ex)
	})
}

func ensureRecord(ctx context.Context, ex postgres.SQLExecutor, subject string) error {
	_, err := ex.ExecContext(ctx, "INSERT INTO admin_profile_presentations(subject_id) VALUES($1) ON CONFLICT DO NOTHING", subject)
	return err
}

func versionString(v int64) string { return strconv.FormatInt(v, 10) }

func nextVersion(r record, expected string) error {
	if expected != versionString(r.Version) || r.Version == math.MaxInt64 {
		return ErrConflict
	}
	return nil
}

func storePending(ctx context.Context, ex postgres.SQLExecutor, subject string, value pending) error {
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > 8192 {
		return ErrInvalid
	}
	_, err = ex.ExecContext(ctx, "UPDATE admin_profile_presentations SET pending=$2::jsonb WHERE subject_id=$1",
		subject, string(raw))
	return err
}

func (c *Component) read(ctx context.Context, binding host.ExternalBinding) (View, error) {
	view := View{Enabled: true, Version: "1"}
	err := c.owned(ctx, binding, func(ctx context.Context, ex postgres.SQLExecutor) error {
		r, err := readRecord(ctx, ex, binding.LocalSubject.String())
		if err != nil {
			return err
		}
		if r.Version == 0 {
			return nil
		}
		if r.Pending.Selector != "" && !time.Now().Before(r.Pending.Until) {
			if err := storePending(ctx, ex, binding.LocalSubject.String(), pending{}); err != nil {
				return err
			}
		}
		view.Version, view.Fields = versionString(r.Version), r.Fields
		r.Binding.LocalSubject = binding.LocalSubject
		if samePins(r.Binding, binding) && r.Updated.Valid {
			view.Available = true
			view.Name, view.Login = r.Snapshot.Name, r.Snapshot.Login
			if c.avatars.validate(r.Snapshot.AvatarURL) {
				view.AvatarURL = r.Snapshot.AvatarURL
			}
			view.UpdatedAt = r.Updated.Time.UTC().Format(time.RFC3339)
		}
		return nil
	})
	return view, err
}

func (c *Component) selectFields(ctx context.Context, binding host.ExternalBinding,
	expected string, fields Fields, authSession string,
) error {
	if fields.Avatar && len(c.avatars.hosts) == 0 {
		return ErrInvalid
	}
	return c.owned(ctx, binding, func(ctx context.Context, ex postgres.SQLExecutor) error {
		subject := binding.LocalSubject.String()
		if err := originalSessionActive(ctx, ex, pending{AuthSession: authSession, Original: binding}); err != nil {
			return err
		}
		if err := ensureRecord(ctx, ex, subject); err != nil {
			return err
		}
		r, err := readRecord(ctx, ex, subject)
		if err != nil {
			return err
		}
		if err = nextVersion(r, expected); err != nil {
			return err
		}
		// Selection changes cancel in-flight refreshes. Saved local canonical
		// names/login/upload and the previous snapshot are untouched.
		_, err = ex.ExecContext(ctx, `UPDATE admin_profile_presentations
 SET use_name=$2,use_login=$3,use_avatar=$4,version=version+1,pending='{}'::jsonb WHERE subject_id=$1`,
			subject, fields.Name, fields.Login, fields.Avatar)
		return err
	})
}
