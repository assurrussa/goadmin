package adminrepo

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/squirrel"
	querybuilder "github.com/assurrussa/outbox/shared/query_builder"
	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"

	datagrid "github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/internal/admintx"
	authcore "github.com/assurrussa/goadmin/internal/auth"
	identity "github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
)

const (
	tableName      = "administrations"
	adminIDColumn  = "a.id"
	fieldUpdatedAt = "updated_at"
	fieldVersion   = "version"
)

var ErrAdminVersionConflict = errors.New("admin data was changed concurrently")

var (
	columns = []string{
		adminIDColumn, "a.subject_id", "a.uuid",
		"COALESCE(bp.username, '') AS username",
		"COALESCE(bp.given_name, '') AS name",
		"COALESCE(bp.family_name, '') AS last_name",
		"i.display_value AS email", "i.verified_at AS confirmed_email_at",
		"a.phone", "a.version", "a.data",
		"a.created_at", "a.updated_at", "a.deleted_at",
	}
	allowedSortFields = map[string]bool{
		"id":         true,
		"username":   true,
		"name":       true,
		"last_name":  true,
		"email":      true,
		"phone":      true, //nolint:goconst // required
		"created_at": true,
	}
	adoptedFields = map[string]datagrid.FieldMapping{
		"id":            datagrid.NewFieldMapping("a.id"),                                        // точное совпадение
		"uuid":          datagrid.NewFieldMapping("a.uuid"),                                      // точное совпадение
		"phone":         datagrid.NewFieldMapping("a.phone"),                                     // точное совпадение
		"username":      datagrid.NewFieldMappingWithOp("bp.username", datagrid.OpLike),          // поиск по подстроке
		"name":          datagrid.NewFieldMappingWithOp("bp.given_name", datagrid.OpLike),        // поиск по подстроке
		"lastName":      datagrid.NewFieldMappingWithOp("bp.family_name", datagrid.OpLike),       // поиск по подстроке
		"email":         datagrid.NewFieldMappingWithOp("i.normalized_value", datagrid.OpLike),   // поиск по подстроке
		"createdAt":     datagrid.NewFieldMapping("a.created_at"),                                // точное совпадение
		"createdAtFrom": datagrid.NewFieldMappingWithOp("a.created_at", datagrid.OpGreaterEqual), // дата от
		"createdAtTo":   datagrid.NewFieldMappingWithOp("a.created_at", datagrid.OpLessEqual),    // дата до
	}
)

type adminRoleNameRow struct {
	AdminID  int64  `db:"admin_id"`
	RoleName string `db:"role_name"`
}

func (r *Repo) GetList(ctx context.Context, filters datagrid.Filtered) ([]models.Admin, int, error) {
	const op = "admin.repo.GetList"

	sqlBuilderCount := outbox.BuilderDollar().
		Select("count(a.id) as total").
		From(tableName + " a").
		Join("auth_subjects s ON s.id = a.subject_id").
		Join("auth_identifiers i ON i.subject_id = a.subject_id AND i.scheme = 'email' AND i.is_primary").
		LeftJoin("auth_basic_profiles bp ON bp.subject_id = a.subject_id")

	search := strings.TrimSpace(strings.ToLower(filters.GetSearch())) + "%"
	sqlBuilderCount = sqlBuilderCount.Where(squirrel.Expr("i.normalized_value LIKE ?", search))
	sqlBuilderCount = datagrid.SQLWherex(sqlBuilderCount, filters, adoptedFields)

	const (
		filterAll         = "all"
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
		case filterAll:
		default:
			sqlBuilderCount = sqlBuilderCount.Where(squirrel.Eq{"deleted_at": nil})
		}
	} else {
		sqlBuilderCount = sqlBuilderCount.Where(squirrel.Eq{"deleted_at": nil})
	}
	if v, ok := filters.GetFields()["emailStatus"].(string); ok {
		switch v {
		case filterConfirmed:
			sqlBuilderCount = sqlBuilderCount.Where(squirrel.Expr("i.verified_at IS NOT NULL"))
		case filterUnconfirmed:
			sqlBuilderCount = sqlBuilderCount.Where(squirrel.Expr("i.verified_at IS NULL"))
		}
	}

	var count int
	if err := admintx.Wrap(r.pgsql.DB()).ScanOnex(ctx, op, &count, sqlBuilderCount); err != nil {
		return nil, 0, fmt.Errorf("%s: error creating account: %w", op, outbox.ErrorTransform(err))
	}

	sqlBuilderList := outbox.BuilderDollar().
		Select(columns...).
		From(tableName + " a").
		Join("auth_subjects s ON s.id = a.subject_id").
		Join("auth_identifiers i ON i.subject_id = a.subject_id AND i.scheme = 'email' AND i.is_primary").
		LeftJoin("auth_basic_profiles bp ON bp.subject_id = a.subject_id")

	sqlBuilderList = datagrid.SQLBuilderx(sqlBuilderList, filters, allowedSortFields)
	sqlBuilderList = datagrid.SQLWherex(sqlBuilderList, filters, adoptedFields)
	sqlBuilderList = sqlBuilderList.Where(squirrel.Expr("i.normalized_value LIKE ?", search))
	if v, ok := filters.GetFields()["status"].(string); ok {
		switch v {
		case filterActive:
			sqlBuilderList = sqlBuilderList.Where(squirrel.Eq{"deleted_at": nil})
		case filterDeleted:
			sqlBuilderList = sqlBuilderList.Where(squirrel.NotEq{"deleted_at": nil})
		case filterAll:
		default:
			sqlBuilderList = sqlBuilderList.Where(squirrel.Eq{"deleted_at": nil})
		}
	} else {
		sqlBuilderList = sqlBuilderList.Where(squirrel.Eq{"deleted_at": nil})
	}
	if v, ok := filters.GetFields()["emailStatus"].(string); ok {
		switch v {
		case filterConfirmed:
			sqlBuilderList = sqlBuilderList.Where(squirrel.Expr("i.verified_at IS NOT NULL"))
		case filterUnconfirmed:
			sqlBuilderList = sqlBuilderList.Where(squirrel.Expr("i.verified_at IS NULL"))
		}
	}

	var data []models.Admin
	if err := admintx.Wrap(r.pgsql.DB()).ScanAllx(ctx, op, &data, sqlBuilderList); err != nil {
		return nil, 0, fmt.Errorf("%s: error get list: %w", op, outbox.ErrorTransform(err))
	}

	return data, count, nil
}

// ListRoleNamesByAdminIDs loads the roles displayed on one admin grid page in
// one query. The order matches goauth's SubjectRoles ordering.
func (r *Repo) ListRoleNamesByAdminIDs(ctx context.Context, adminIDs []int64) (map[int64][]string, error) {
	if len(adminIDs) == 0 {
		return map[int64][]string{}, nil
	}

	query := outbox.BuilderDollar().
		Select(adminIDColumn+" AS admin_id", "r.name AS role_name").
		From("administrations a").
		Join("auth_subject_roles sr ON sr.subject_id = a.subject_id").
		Join("auth_roles r ON r.id = sr.role_id").
		Where(squirrel.Eq{adminIDColumn: adminIDs}).
		OrderBy(adminIDColumn, "r.is_system DESC", "r.name", "r.id")
	var rows []adminRoleNameRow
	if err := admintx.Wrap(r.pgsql.DB()).ScanAllx(ctx, "admin.repo.ListRoleNamesByAdminIDs", &rows, query); err != nil {
		return nil, fmt.Errorf("list admin roles: %w", outbox.ErrorTransform(err))
	}

	roles := make(map[int64][]string, len(adminIDs))
	for _, row := range rows {
		roles[row.AdminID] = append(roles[row.AdminID], row.RoleName)
	}
	return roles, nil
}

func (r *Repo) GetByID(ctx context.Context, id int64) (models.Admin, error) {
	const op = "admin.repo.GetByID"

	if id <= 0 {
		return models.Admin{}, fmt.Errorf("%s: invalid id", op)
	}

	sqlBuilder := outbox.BuilderDollar().
		Select(columns...).
		From(tableName + " a").
		Join("auth_subjects s ON s.id = a.subject_id").
		Join("auth_identifiers i ON i.subject_id = a.subject_id AND i.scheme = 'email' AND i.is_primary").
		LeftJoin("auth_basic_profiles bp ON bp.subject_id = a.subject_id").
		Where(squirrel.Eq{"a.id": id}).
		Limit(1)

	var adm models.Admin
	if err := admintx.Wrap(r.pgsql.DB()).ScanOnex(ctx, op, &adm, sqlBuilder); err != nil {
		if pgxscan.NotFound(err) {
			return models.Admin{}, nil
		}

		return models.Admin{}, fmt.Errorf("%s: error get: %w", op, outbox.ErrorTransform(err))
	}

	return adm, nil
}

func (r *Repo) GetByUUID(ctx context.Context, id identity.UserID) (models.Admin, error) {
	const op = "admin.repo.GetByUUID"

	if id.IsZero() {
		return models.Admin{}, fmt.Errorf("%s: invalid id", op)
	}

	sqlBuilder := outbox.BuilderDollar().
		Select(columns...).
		From(tableName + " a").
		Join("auth_subjects s ON s.id = a.subject_id").
		Join("auth_identifiers i ON i.subject_id = a.subject_id AND i.scheme = 'email' AND i.is_primary").
		LeftJoin("auth_basic_profiles bp ON bp.subject_id = a.subject_id").
		Where(squirrel.Eq{"a.uuid": id}).
		Limit(1)

	var adm models.Admin
	if err := admintx.Wrap(r.pgsql.DB()).ScanOnex(ctx, op, &adm, sqlBuilder); err != nil {
		if pgxscan.NotFound(err) {
			return models.Admin{}, nil
		}

		return models.Admin{}, fmt.Errorf("%s: error get: %w", op, outbox.ErrorTransform(err))
	}

	return adm, nil
}

func (r *Repo) GetBySubjectID(ctx context.Context, subjectID authcore.SubjectID) (models.Admin, error) {
	const op = "admin.repo.GetBySubjectID"

	if subjectID.IsZero() {
		return models.Admin{}, fmt.Errorf("%s: invalid subject id", op)
	}

	sqlBuilder := outbox.BuilderDollar().
		Select(columns...).
		From(tableName + " a").
		Join("auth_subjects s ON s.id = a.subject_id").
		Join("auth_identifiers i ON i.subject_id = a.subject_id AND i.scheme = 'email' AND i.is_primary").
		LeftJoin("auth_basic_profiles bp ON bp.subject_id = a.subject_id").
		Where(squirrel.Eq{"a.subject_id": subjectID, "a.deleted_at": nil}).
		Limit(1)

	var adm models.Admin
	if err := admintx.Wrap(r.pgsql.DB()).ScanOnex(ctx, op, &adm, sqlBuilder); err != nil {
		if pgxscan.NotFound(err) {
			return models.Admin{}, nil
		}

		return models.Admin{}, fmt.Errorf("%s: error get: %w", op, outbox.ErrorTransform(err))
	}

	return adm, nil
}

func (r *Repo) DeleteByID(ctx context.Context, id int64) error {
	const op = "admin.repo.DeleteByID"

	if id <= 0 {
		return fmt.Errorf("%s: invalid id", op)
	}

	sqlBuilder := outbox.BuilderDollar().
		Delete(tableName).
		Where(squirrel.Eq{"id": id})

	if _, err := admintx.Wrap(r.pgsql.DB()).Execx(ctx, op, sqlBuilder); err != nil {
		return fmt.Errorf("%s: error delete: %w", op, outbox.ErrorTransform(err))
	}

	return nil
}

func (r *Repo) SoftDeleteByID(ctx context.Context, id int64) error {
	const op = "admin.repo.SoftDeleteByID"

	if id <= 0 {
		return fmt.Errorf("%s: invalid id", op)
	}

	sqlBuilder := outbox.BuilderDollar().
		Update(tableName).
		SetMap(querybuilder.Eq{
			"deleted_at": time.Now(),
		}).Where(squirrel.Eq{"id": id, "deleted_at": nil}).
		Where(`NOT EXISTS (
			SELECT 1 FROM auth_subject_roles sr
			JOIN auth_roles role ON role.id = sr.role_id
			WHERE sr.subject_id = administrations.subject_id AND role.slug = 'super_admin'
		)`)

	tag, err := admintx.Wrap(r.pgsql.DB()).Execx(ctx, op, sqlBuilder)
	if err != nil {
		return fmt.Errorf("%s: error soft delete: %w", op, outbox.ErrorTransform(err))
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, pgx.ErrNoRows)
	}

	return nil
}

func (r *Repo) UpdateAdmin(ctx context.Context, id int64, admin models.Admin) error {
	const op = "admin.repo.UpdateAdmin"

	builder := outbox.BuilderDollar().
		Update(tableName).
		SetMap(querybuilder.Eq{
			"data":         admin.Data,
			fieldVersion:   squirrel.Expr("version + 1"),
			fieldUpdatedAt: time.Now(),
		}).Where(squirrel.Eq{"id": id, fieldVersion: admin.Version, "deleted_at": nil})

	tag, err := admintx.Wrap(r.pgsql.DB()).Execx(ctx, op, builder)
	if err != nil {
		return fmt.Errorf("error update account: %w", outbox.ErrorTransform(err))
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, ErrAdminVersionConflict)
	}

	return nil
}

func (r *Repo) UpdatePhone(ctx context.Context, id int64, phone *int64) error {
	const op = "admin.repo.UpdatePhone"

	builder := outbox.BuilderDollar().
		Update(tableName).
		SetMap(querybuilder.Eq{
			"phone":        phone,
			fieldVersion:   squirrel.Expr("version + 1"),
			fieldUpdatedAt: time.Now(),
		}).Where(squirrel.Eq{"id": id})

	if _, err := admintx.Wrap(r.pgsql.DB()).Execx(ctx, op, builder); err != nil {
		return fmt.Errorf("error update: %w", outbox.ErrorTransform(err))
	}

	return nil
}

func (r *Repo) UpdatePreview(ctx context.Context, admin models.Admin, fileID int64) error {
	const op = "admin.repo.UpdatePreview"

	expectedPreviewID := admin.GetPreviewFileID()
	adminData := *admin.GetData()
	if fileID <= 0 {
		adminData.PreviewFileID = nil
	} else {
		adminData.PreviewFileID = &fileID
	}

	builder := outbox.BuilderDollar().
		Update(tableName).
		SetMap(querybuilder.Eq{
			"data":         &adminData,
			fieldVersion:   squirrel.Expr("version + 1"),
			fieldUpdatedAt: time.Now(),
		}).Where(squirrel.Eq{"id": admin.ID, fieldVersion: admin.Version, "deleted_at": nil}).
		Where(squirrel.Expr("COALESCE(data->>'previewFileId', '0') = ?", strconv.FormatInt(expectedPreviewID, 10)))

	tag, err := admintx.Wrap(r.pgsql.DB()).Execx(ctx, op, builder)
	if err != nil {
		return fmt.Errorf("error update: %w", outbox.ErrorTransform(err))
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, ErrAdminVersionConflict)
	}

	return nil
}
