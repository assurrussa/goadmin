package jobsbatchesrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	querybuilder "github.com/assurrussa/outbox/shared/query_builder"
	"github.com/georgysavva/scany/v2/pgxscan"

	datagrid "github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
)

const (
	tableName = "jobs_batches"
)

var (
	ErrNoJobs = errors.New("no jobs found")

	columns = []string{
		"id", "name", "total_jobs", "pending_jobs", "failed_jobs", "failed_job_ids", "options", //nolint:goconst // required
		"cancelled_at", "created_at", "finished_at",
	}

	allowedSortFields = map[string]bool{
		"id": true,
	}
	adoptedFields = map[string]datagrid.FieldMapping{
		"id":   datagrid.NewFieldMapping("id"),   // точное совпадение
		"name": datagrid.NewFieldMapping("name"), // точное совпадение
	}
)

func (r *Repo) GetList(ctx context.Context, filters datagrid.Filtered) ([]outbox.JobBatchesModel, int, error) {
	const op = "jobs_batches.repo.GetList"

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

	var data []outbox.JobBatchesModel
	if err := r.pgsql.DB().ScanAllx(ctx, op, &data, sqlBuilderList); err != nil {
		return nil, 0, fmt.Errorf("%s: error get list: %w", op, outbox.ErrorTransform(err))
	}

	return data, count, nil
}

func (r *Repo) Create(ctx context.Context, job outbox.JobBatchesModel) (int64, error) {
	const op = "jobs_failed.repo.Create"

	builder := outbox.BuilderDollar().
		Insert(tableName).
		Suffix("RETURNING id").
		SetMap(querybuilder.Eq{
			"name":           job.Name,
			"total_jobs":     job.TotalJobs,
			"pending_jobs":   job.PendingJobs,
			"failed_jobs":    job.FailedJobs,
			"failed_job_ids": job.FailedJobIDs,
			"options":        job.Options,
			"cancelled_at":   job.CancelledAt,
			"created_at":     job.CreatedAt,
			"finished_at":    job.FinishedAt,
		})

	var lastID int64
	if err := r.pgsql.DB().Getx(ctx, op, &lastID, builder); err != nil {
		return 0, fmt.Errorf("error creating: %w", outbox.ErrorTransform(err))
	}

	return lastID, nil
}

func (r *Repo) GetByID(ctx context.Context, id int64) (outbox.JobBatchesModel, error) {
	const op = "jobs_batches.repo.GetByID"

	if id <= 0 {
		return outbox.JobBatchesModel{}, fmt.Errorf("%s: invalid id", op)
	}

	sqlBuilder := outbox.BuilderDollar().
		Select(columns...).
		From(tableName).
		Where(squirrel.Eq{"id": id}).
		Limit(1)

	var adm outbox.JobBatchesModel
	if err := r.pgsql.DB().ScanOnex(ctx, op, &adm, sqlBuilder); err != nil {
		if pgxscan.NotFound(err) {
			return outbox.JobBatchesModel{}, errors.Join(err, ErrNoJobs)
		}

		return outbox.JobBatchesModel{}, fmt.Errorf("%s: error get: %w", op, outbox.ErrorTransform(err))
	}

	return adm, nil
}

func (r *Repo) CountLight(ctx context.Context) (int64, error) {
	const op = "jobs_batches.repo.CountLight"

	count, err := outbox.CountRowsForTable(ctx, r.pgsql.DB(), tableName)
	if err != nil {
		return 0, fmt.Errorf("%s: CountRowsForTable: %w", op, outbox.ErrorTransform(err))
	}

	return count, nil
}

func (r *Repo) CountExact(ctx context.Context) (int64, error) {
	const op = "jobs_batches.repo.CountExact"

	sqlBuilderCount := outbox.BuilderDollar().
		Select("count(id) as total").
		From(tableName)

	var count int64
	if err := r.pgsql.DB().ScanOnex(ctx, op, &count, sqlBuilderCount); err != nil {
		return 0, fmt.Errorf("%s: error get: %w", op, outbox.ErrorTransform(err))
	}

	return count, nil
}

func (r *Repo) Delete(ctx context.Context, jobID int64) (int64, error) {
	const op = "jobs_batches.repo.Delete"

	if jobID <= 0 {
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
