package jobsrepo

import (
	"context"
	"fmt"

	"github.com/Masterminds/squirrel"

	datagrid "github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
)

const (
	tableName         = "jobs"
	columnAvailableAt = "available_at"
	columnCreatedAt   = "created_at"
)

var (
	columns = []string{
		"id", "queue", "name", "schema_version", "payload", "attempts", "reserved_at", "lease_token", //nolint:goconst // required
		"deduplication_key", columnAvailableAt, columnCreatedAt,
	}

	allowedSortFields = map[string]bool{
		"id":              true,
		"queue":           true,
		"name":            true,
		"attempts":        true,
		columnCreatedAt:   true,
		columnAvailableAt: true,
		"reserved_at":     true,
	}
	adoptedFields = map[string]datagrid.FieldMapping{
		"id":            datagrid.NewFieldMapping("id"),                                           // точное совпадение
		"queue":         datagrid.NewFieldMapping("queue"),                                        // точное совпадение
		"name":          datagrid.NewFieldMapping("name"),                                         // точное совпадение
		"createdAt":     datagrid.NewFieldMapping(columnCreatedAt),                                // точное совпадение
		"createdAtFrom": datagrid.NewFieldMappingWithOp(columnCreatedAt, datagrid.OpGreaterEqual), // дата от
		"createdAtTo":   datagrid.NewFieldMappingWithOp(columnCreatedAt, datagrid.OpLessEqual),    // дата до
	}

	sortAliases = map[string]string{
		"createdAt":   columnCreatedAt,
		"availableAt": columnAvailableAt,
		"reservedAt":  "reserved_at",
	}
)

type sortAliasFilter struct {
	datagrid.Filtered
	sortBy string
}

func (f sortAliasFilter) GetSortBy() string {
	if f.sortBy != "" {
		return f.sortBy
	}

	return f.Filtered.GetSortBy()
}

func applySortAliases(filters datagrid.Filtered, aliases map[string]string) datagrid.Filtered {
	if mapped, ok := aliases[filters.GetSortBy()]; ok {
		return sortAliasFilter{Filtered: filters, sortBy: mapped}
	}

	return filters
}

func (r *Repo) GetList(ctx context.Context, filters datagrid.Filtered) ([]outbox.JobModel, int, error) {
	const op = "jobs.repo.GetList"

	filters = applySortAliases(filters, sortAliases)

	sqlBuilderCount := outbox.BuilderDollar().
		Select("count(id) as total").
		From(tableName)

	sqlBuilderCount = datagrid.SQLWherex(sqlBuilderCount, filters, adoptedFields)

	var count int
	if err := r.pgsql.DB().ScanOnex(ctx, op, &count, sqlBuilderCount); err != nil {
		return nil, 0, fmt.Errorf("%s: error creating account: %w", op, outbox.ErrorTransform(err))
	}

	sqlBuilderList := outbox.BuilderDollar().
		Select(columns...).
		From(tableName)

	sqlBuilderList = datagrid.SQLBuilderx(sqlBuilderList, filters, allowedSortFields)
	sqlBuilderList = datagrid.SQLWherex(sqlBuilderList, filters, adoptedFields)

	var data []outbox.JobModel
	if err := r.pgsql.DB().ScanAllx(ctx, op, &data, sqlBuilderList); err != nil {
		return nil, 0, fmt.Errorf("%s: error get list: %w", op, outbox.ErrorTransform(err))
	}

	return data, count, nil
}

func (r *Repo) Create(ctx context.Context, job outbox.JobModel) (outbox.JobID, error) {
	if job.ID.IsZero() {
		job.ID = outbox.NewJobID()
	}

	return r.repo.Create(ctx, job)
}

func (r *Repo) GetByID(ctx context.Context, id outbox.JobID) (outbox.JobModel, error) {
	return r.repo.GetByID(ctx, id)
}

func (r *Repo) All(ctx context.Context) ([]outbox.JobModel, error) {
	return r.repo.All(ctx)
}

func (r *Repo) CountLight(ctx context.Context) (int64, error) {
	return r.repo.CountLight(ctx)
}

func (r *Repo) CountExact(ctx context.Context) (int64, error) {
	const op = "jobs.repo.CountExact"

	sqlBuilderCount := outbox.BuilderDollar().
		Select("count(id) as total").
		From(tableName)

	var count int64
	if err := r.pgsql.DB().ScanOnex(ctx, op, &count, sqlBuilderCount); err != nil {
		return 0, fmt.Errorf("%s: error get: %w", op, outbox.ErrorTransform(err))
	}

	return count, nil
}

func (r *Repo) DeleteJob(ctx context.Context, jobID outbox.JobID) (int64, error) {
	const op = "jobs.repo.DeleteJob"

	if jobID.IsZero() {
		return 0, fmt.Errorf("%s: invalid id", op)
	}

	sqlBuilder := outbox.BuilderDollar().
		Delete(tableName).
		Where(squirrel.Eq{"id": jobID})

	result, err := r.pgsql.DB().Execx(ctx, op, sqlBuilder)
	if err != nil {
		return 0, fmt.Errorf("%s: error deleted: %w", op, outbox.ErrorTransform(err))
	}

	return result.RowsAffected(), nil
}
