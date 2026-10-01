package jobsfailedrepo

import (
	"context"
	"fmt"

	datagrid "github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
)

const (
	tableName       = "jobs_failed"
	columnCreatedAt = "created_at"
	columnFailedAt  = "failed_at"
)

var (
	columns = []string{
		"id", "job_id", "connection", "queue", "name", "schema_version", "payload", "reason", "exception",
		columnFailedAt, columnCreatedAt,
	}

	allowedSortFields = map[string]bool{
		"id":            true,
		columnFailedAt:  true,
		columnCreatedAt: true,
	}
	adoptedFields = map[string]datagrid.FieldMapping{
		"id":   datagrid.NewFieldMapping("id"),   // точное совпадение
		"name": datagrid.NewFieldMapping("name"), // точное совпадение
	}

	sortAliases = map[string]string{
		"failedAt":  columnFailedAt,
		"createdAt": columnCreatedAt,
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

func (r *Repo) GetList(ctx context.Context, filters datagrid.Filtered) ([]outbox.JobFailedModel, int, error) {
	const op = "jobs_failed.repo.GetList"

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

	var data []outbox.JobFailedModel
	if err := r.pgsql.DB().ScanAllx(ctx, op, &data, sqlBuilderList); err != nil {
		return nil, 0, fmt.Errorf("%s: error get list: %w", op, outbox.ErrorTransform(err))
	}

	return data, count, nil
}

func (r *Repo) Create(ctx context.Context, job outbox.JobFailedModel) (outbox.JobID, error) {
	if job.ID.IsZero() {
		job.ID = outbox.NewJobID()
	}

	return r.repo.Create(ctx, job)
}

func (r *Repo) All(ctx context.Context) ([]outbox.JobFailedModel, error) {
	return r.repo.All(ctx)
}

func (r *Repo) GetByID(ctx context.Context, id outbox.JobID) (outbox.JobFailedModel, error) {
	return r.repo.GetByID(ctx, id)
}

func (r *Repo) CountLight(ctx context.Context) (int64, error) {
	return r.repo.CountLight(ctx)
}

func (r *Repo) CountExact(ctx context.Context) (int64, error) {
	const op = "jobs_failed.repo.CountExact"

	sqlBuilderCount := outbox.BuilderDollar().
		Select("count(id) as total").
		From(tableName)

	var count int64
	if err := r.pgsql.DB().ScanOnex(ctx, op, &count, sqlBuilderCount); err != nil {
		return 0, fmt.Errorf("%s: error get: %w", op, outbox.ErrorTransform(err))
	}

	return count, nil
}

func (r *Repo) Delete(ctx context.Context, jobID outbox.JobID) (int64, error) {
	return r.repo.Delete(ctx, jobID)
}
