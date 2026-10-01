package adminrepo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/assurrussa/goauth"
	"github.com/georgysavva/scany/v2/pgxscan"

	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/internal/admintx"
	identity "github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
)

// ProvisionAccount creates the host-owned admin membership for an already
// provisioned canonical account. Canonical identifiers, profiles, and
// credentials remain exclusively owned by goauth Runtime.
func (r *Repo) ProvisionAccount(
	ctx context.Context,
	account goauth.Account,
	publicID identity.UserID,
) (models.Admin, error) {
	id, err := r.ProvisionAdminMembership(ctx, account, publicID)
	if err != nil {
		return models.Admin{}, err
	}

	created, err := r.GetByID(ctx, id)
	if err != nil {
		return models.Admin{}, fmt.Errorf("provision admin membership: load result: %w", err)
	}

	return created, nil
}

// ProvisionAdminMembership is the narrow write contract used by the goadmin
// auth adapter and trusted seed path.
func (r *Repo) ProvisionAdminMembership(
	ctx context.Context,
	account goauth.Account,
	publicID identity.UserID,
) (int64, error) {
	if account.IsZero() {
		return 0, goauth.ErrAccountNotFound
	}
	if publicID.IsZero() {
		return 0, errors.New("invalid admin public id")
	}

	existing, err := r.GetBySubjectID(ctx, account.Subject.ID)
	if err != nil {
		return 0, fmt.Errorf("provision admin membership: get existing membership: %w", err)
	}
	if existing.ID != 0 {
		return existing.ID, nil
	}

	now := time.Now().UTC()
	createdAt := account.Subject.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = now
	}
	builder := outbox.BuilderDollar().
		Insert(tableName).
		Columns("subject_id", "uuid", "version", "data", "created_at", "updated_at").
		Values(account.Subject.ID, publicID, 0, &models.AdminData{}, createdAt, now).
		Suffix("ON CONFLICT DO NOTHING RETURNING id")

	var id int64
	if err := admintx.Wrap(r.pgsql.DB()).Getx(ctx, "admin.repo.ProvisionAccount", &id, builder); err != nil {
		if !pgxscan.NotFound(err) {
			return 0, fmt.Errorf("provision admin membership: %w", outbox.ErrorTransform(err))
		}
		// A concurrent insert can conflict on subject_id or uuid. The former is
		// idempotent only while the membership is active; the latter must not
		// hand another subject's membership to this account.
		const liveRecordColumn = "deleted_at"
		lookup := outbox.BuilderDollar().Select("id").From(tableName).
			Where(squirrel.Eq{"subject_id": account.Subject.ID, liveRecordColumn: nil}).Limit(1)
		if lookupErr := admintx.Wrap(r.pgsql.DB()).Getx(ctx, "admin.repo.ProvisionAccount.lookup", &id, lookup); lookupErr != nil {
			if pgxscan.NotFound(lookupErr) {
				return 0, goauth.ErrMembershipDenied
			}
			return 0, fmt.Errorf("provision admin membership: lookup conflict: %w", lookupErr)
		}
	}

	return id, nil
}
