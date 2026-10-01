package userrepo

import (
	"context"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	querybuilder "github.com/assurrussa/outbox/shared/query_builder"
	"github.com/georgysavva/scany/v2/pgxscan"

	datagrid "github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	authcore "github.com/assurrussa/goadmin/internal/auth"
	identity "github.com/assurrussa/goadmin/internal/identity"
)

const tableName = "users"

var (
	columns = []string{
		"u.id", "u.subject_id", "u.uuid",
		"i.display_value AS email", "bp.username", "bp.given_name AS name", "bp.family_name AS last_name",
		"u.phone", "u.bio", "u.version", "u.data",
		"u.telegram_chat_id", "u.telegram_username",
		"i.verified_at AS confirmed_email_at", "u.confirmed_phone_at", "u.created_at", "u.updated_at", "u.deleted_at",
	}
	allowedSortFields = map[string]bool{
		"id":                 true,
		"email":              true,
		"created_at":         true,
		"confirmed_email_at": true,
		"confirmed_phone_at": true,
	}
	adoptedFields = map[string]datagrid.FieldMapping{
		"id":                datagrid.NewFieldMapping("u.id"),
		"uuid":              datagrid.NewFieldMapping("u.uuid"),
		"telegram_chat_id":  datagrid.NewFieldMapping("u.telegram_chat_id"),
		"telegram_username": datagrid.NewFieldMappingWithOp("u.telegram_username", datagrid.OpLike),
		"email":             datagrid.NewFieldMappingWithOp("i.normalized_value", datagrid.OpLike),
		"name":              datagrid.NewFieldMappingWithOp("bp.given_name", datagrid.OpLike),
		"lastName":          datagrid.NewFieldMappingWithOp("bp.family_name", datagrid.OpLike),
		"createdAt":         datagrid.NewFieldMapping("u.created_at"),
		"deletedAt":         datagrid.NewFieldMapping("u.deleted_at"),
		"createdAtFrom":     datagrid.NewFieldMappingWithOp("u.created_at", datagrid.OpGreaterEqual),
		"createdAtTo":       datagrid.NewFieldMappingWithOp("u.created_at", datagrid.OpLessEqual),
	}
)

func (r *Repo) GetList(ctx context.Context, filters datagrid.Filtered) ([]authcore.Profile, int, error) {
	const op = "user.repo.GetList"

	sqlBuilderCount := outbox.BuilderDollar().
		Select("count(u.id) as total").
		From(tableName + " u").
		Join("auth_subjects s ON s.id = u.subject_id").
		Join("auth_identifiers i ON i.subject_id = u.subject_id AND i.scheme = 'email' AND i.is_primary").
		LeftJoin("auth_basic_profiles bp ON bp.subject_id = u.subject_id")

	sqlBuilderCount = datagrid.SQLWherex(sqlBuilderCount, filters, adoptedFields)

	const (
		filterActive      = "active"
		filterDeleted     = "deleted"
		filterConfirmed   = "confirmed"
		filterUnconfirmed = "unconfirmed"
	)

	if v, ok := filters.GetFields()["status"].(string); ok {
		switch v {
		case filterActive:
			sqlBuilderCount = sqlBuilderCount.Where(squirrel.Eq{"deleted_at": nil}) //nolint:goconst // required
		case filterDeleted:
			sqlBuilderCount = sqlBuilderCount.Where(squirrel.NotEq{"deleted_at": nil})
		}
	}
	if v, ok := filters.GetFields()["emailStatus"].(string); ok {
		switch v {
		case filterConfirmed:
			sqlBuilderCount = sqlBuilderCount.Where(squirrel.Expr("i.verified_at IS NOT NULL"))
		case filterUnconfirmed:
			sqlBuilderCount = sqlBuilderCount.Where(squirrel.Expr("i.verified_at IS NULL"))
		}
	}
	if v, ok := filters.GetFields()["phoneStatus"].(string); ok {
		switch v {
		case filterConfirmed:
			sqlBuilderCount = sqlBuilderCount.Where(squirrel.Expr("u.confirmed_phone_at IS NOT NULL"))
		case filterUnconfirmed:
			sqlBuilderCount = sqlBuilderCount.Where(squirrel.Expr("u.confirmed_phone_at IS NULL"))
		}
	}

	var count int
	if err := r.pgsql.DB().ScanOnex(ctx, op, &count, sqlBuilderCount); err != nil {
		return nil, 0, fmt.Errorf("%s: error count: %w", op, outbox.ErrorTransform(err))
	}

	sqlBuilderList := outbox.BuilderDollar().
		Select(columns...).
		From(tableName + " u").
		Join("auth_subjects s ON s.id = u.subject_id").
		Join("auth_identifiers i ON i.subject_id = u.subject_id AND i.scheme = 'email' AND i.is_primary").
		LeftJoin("auth_basic_profiles bp ON bp.subject_id = u.subject_id")

	sqlBuilderList = datagrid.SQLBuilderx(sqlBuilderList, filters, allowedSortFields)
	sqlBuilderList = datagrid.SQLWherex(sqlBuilderList, filters, adoptedFields)

	if v, ok := filters.GetFields()["status"].(string); ok {
		switch v {
		case filterActive:
			sqlBuilderList = sqlBuilderList.Where(squirrel.Eq{"deleted_at": nil})
		case filterDeleted:
			sqlBuilderList = sqlBuilderList.Where(squirrel.NotEq{"deleted_at": nil})
		}
	}
	if v, ok := filters.GetFields()["emailStatus"].(string); ok {
		switch v {
		case filterConfirmed:
			sqlBuilderList = sqlBuilderList.Where(squirrel.Expr("i.verified_at IS NOT NULL"))
		case filterUnconfirmed:
			sqlBuilderList = sqlBuilderList.Where(squirrel.Expr("i.verified_at IS NULL"))
		}
	}
	if v, ok := filters.GetFields()["phoneStatus"].(string); ok {
		switch v {
		case filterConfirmed:
			sqlBuilderList = sqlBuilderList.Where(squirrel.Expr("u.confirmed_phone_at IS NOT NULL"))
		case filterUnconfirmed:
			sqlBuilderList = sqlBuilderList.Where(squirrel.Expr("u.confirmed_phone_at IS NULL"))
		}
	}

	var data []authcore.Profile
	if err := r.pgsql.DB().ScanAllx(ctx, op, &data, sqlBuilderList); err != nil {
		return nil, 0, fmt.Errorf("%s: error get list: %w", op, outbox.ErrorTransform(err))
	}
	return data, count, nil
}

func (r *Repo) GetByID(ctx context.Context, id int64) (authcore.Profile, error) {
	const op = "user.repo.GetByID"

	if id <= 0 {
		return authcore.Profile{}, fmt.Errorf("%s: invalid id", op)
	}

	return r.getUserByCondition(ctx, op, squirrel.Eq{"u.id": id})
}

func (r *Repo) GetByUUID(ctx context.Context, id identity.UserID) (authcore.Profile, error) {
	const op = "user.repo.GetByUUID"

	if err := id.Validate(); err != nil {
		return authcore.Profile{}, fmt.Errorf("%s: invalid uuid: %w", op, err)
	}

	return r.getUserByCondition(ctx, op, squirrel.Eq{"u.uuid": id})
}

func (r *Repo) getUserByCondition(ctx context.Context, op string, eq squirrel.Eq) (authcore.Profile, error) {
	sqlBuilder := outbox.BuilderDollar().
		Select(columns...).
		From(tableName + " u").
		Join("auth_subjects s ON s.id = u.subject_id").
		Join("auth_identifiers i ON i.subject_id = u.subject_id AND i.scheme = 'email' AND i.is_primary").
		LeftJoin("auth_basic_profiles bp ON bp.subject_id = u.subject_id").
		Where(eq).
		Limit(1)

	var user authcore.Profile
	if err := r.pgsql.DB().ScanOnex(ctx, op, &user, sqlBuilder); err != nil {
		if pgxscan.NotFound(err) {
			return authcore.Profile{}, nil
		}

		return authcore.Profile{}, fmt.Errorf("%s: error get: %w", op, outbox.ErrorTransform(err))
	}

	return user, nil
}

func (r *Repo) DeleteByID(ctx context.Context, id int64) error {
	const op = "user.repo.DeleteByID"

	if id <= 0 {
		return fmt.Errorf("%s: invalid id", op)
	}

	sqlBuilder := outbox.BuilderDollar().
		Delete(tableName).
		Where(squirrel.Eq{"id": id})

	if _, err := r.pgsql.DB().Execx(ctx, op, sqlBuilder); err != nil {
		return fmt.Errorf("%s: error deleted: %w", op, outbox.ErrorTransform(err))
	}

	return nil
}

func (r *Repo) SoftDeleteByID(ctx context.Context, id int64) error {
	const op = "user.repo.SoftDeleteByID"

	if id <= 0 {
		return fmt.Errorf("%s: invalid id", op)
	}

	sqlBuilder := outbox.BuilderDollar().
		Update(tableName).
		SetMap(squirrel.Eq{
			"deleted_at": time.Now(),
		}).Where(squirrel.Eq{"id": id})

	if _, err := r.pgsql.DB().Execx(ctx, op, sqlBuilder); err != nil {
		return fmt.Errorf("%s: error soft delete: %w", op, outbox.ErrorTransform(err))
	}

	return nil
}

func (r *Repo) RestoreByID(ctx context.Context, id int64) error {
	const op = "user.repo.RestoreByID"

	if id <= 0 {
		return fmt.Errorf("%s: invalid id", op)
	}

	sqlBuilder := outbox.BuilderDollar().
		Update(tableName).
		SetMap(squirrel.Eq{
			"deleted_at": nil,
			"updated_at": time.Now(), //nolint:goconst // required
		}).Where(squirrel.Eq{"id": id})

	if _, err := r.pgsql.DB().Execx(ctx, op, sqlBuilder); err != nil {
		return fmt.Errorf("%s: error restore: %w", op, outbox.ErrorTransform(err))
	}

	return nil
}

func (r *Repo) GetBySubjectID(ctx context.Context, subjectID authcore.SubjectID) (authcore.Profile, error) {
	if subjectID.IsZero() {
		return authcore.Profile{}, nil
	}

	return r.getUserByCondition(ctx, "user.repo.GetBySubjectID", squirrel.Eq{"u.subject_id": subjectID})
}

func (r *Repo) Update(ctx context.Context, id int64, user authcore.Profile) error {
	const op = "user.repo.Update"

	builder := outbox.BuilderDollar().
		Update(tableName).
		SetMap(querybuilder.Eq{
			"bio":        user.Bio,
			"data":       user.Data,
			"version":    squirrel.Expr("version + 1"),
			"updated_at": time.Now(),
		}).Where(squirrel.Eq{"id": id})

	if _, err := r.pgsql.DB().Execx(ctx, op, builder); err != nil {
		return fmt.Errorf("error update: %w", outbox.ErrorTransform(err))
	}

	return nil
}

func (r *Repo) UpdatePhone(ctx context.Context, id int64, phone *int64) error {
	const op = "user.repo.UpdatePhone"

	builder := outbox.BuilderDollar().
		Update(tableName).
		SetMap(querybuilder.Eq{
			"phone":      phone,
			"version":    squirrel.Expr("version + 1"),
			"updated_at": time.Now(),
		}).Where(squirrel.Eq{"id": id})

	if _, err := r.pgsql.DB().Execx(ctx, op, builder); err != nil {
		return fmt.Errorf("error update: %w", outbox.ErrorTransform(err))
	}

	return nil
}
