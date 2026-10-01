package outbox

import (
	"context"
	"time"

	outboxrepositories "github.com/assurrussa/outbox/backends/pgsql/repositories"
	outboxjobsfailedrepo "github.com/assurrussa/outbox/backends/pgsql/repositories/jobsfailedrepo"
	outboxjobsrepo "github.com/assurrussa/outbox/backends/pgsql/repositories/jobsrepo"
	pgsql "github.com/assurrussa/outbox/backends/pgsql/storage"
	"github.com/assurrussa/outbox/outbox"
	"github.com/assurrussa/outbox/outbox/logger"
	outboxmodels "github.com/assurrussa/outbox/outbox/models"
	"github.com/assurrussa/outbox/shared/sharederrors"
)

type (
	StoragePgsqlFnCallback         = pgsql.FnCallback
	StoragePgsqlClient             = pgsql.Client
	StoragePgsqlDBEngine           = pgsql.DBEngine
	StoragePgsqlTransactor         = pgsql.Transactor
	StoragePgsqlTxManager          = pgsql.TxManager
	StoragePgsqlSqlizer            = pgsql.Sqlizer
	StoragePgsqlPinger             = pgsql.Pinger
	StoragePgsqlNamedExecer        = pgsql.NamedExecer
	StoragePgsqlNamedExecerSqlizer = pgsql.NamedExecerSqlizer
	StoragePgsqlQueryExecer        = pgsql.QueryExecer
	StoragePgsqlQueryExecerSqlizer = pgsql.QueryExecerSqlizer
	StoragePgsqlBatcherExtended    = pgsql.BatcherExtended
	StoragePgsqlSQLExecer          = pgsql.SQLExecer
	StoragePgsqlDBPgxEnginePool    = pgsql.DBPgxEnginePool
	StoragePgsqlConfig             = pgsql.PSQLConfig
)

type (
	Stats                = outbox.Stats
	Job                  = outbox.Job
	Putter               = outbox.Putter
	Transactor           = outbox.Transactor
	JobsRepository       = outbox.JobsRepository
	JobsStatRepository   = outbox.JobsStatRepository
	JobsFailedRepository = outbox.JobsFailedRepository
	Logger               = logger.Logger
)

type (
	Service          = outbox.Service
	OptOptionsSetter = outbox.OptOptionsSetter
	DefaultJob       = outbox.DefaultJob
	QueueStats       = outbox.QueueStats
	JobModel         = outboxmodels.Job
	JobFailedModel   = outboxmodels.JobFailed
	JobBatchesModel  = outboxmodels.JobBatches
	PgsqlJobsRepo    = outboxjobsrepo.Repo
	PgsqlFailedRepo  = outboxjobsfailedrepo.Repo
)

var ErrNoJobs = sharederrors.ErrNoJobs

func NewPgsqlJobsRepo(db StoragePgsqlClient) *PgsqlJobsRepo {
	return outboxjobsrepo.Must(outboxjobsrepo.NewOptions(db))
}

func NewPgsqlJobsFailedRepo(db StoragePgsqlClient) *PgsqlFailedRepo {
	return outboxjobsfailedrepo.Must(outboxjobsfailedrepo.NewOptions(db))
}

func CountRowsForTable(ctx context.Context, storage StoragePgsqlNamedExecer, tableName string) (int64, error) {
	return outboxrepositories.CountRowsForTable(ctx, storage, tableName)
}

func New(options ...OptOptionsSetter) (*Service, error) {
	return outbox.New(options...)
}

func WithWorkers(workers int) OptOptionsSetter {
	return outbox.WithWorkers(workers)
}

func WithIdleTime(idleTime time.Duration) OptOptionsSetter {
	return outbox.WithIdleTime(idleTime)
}

func WithReserveFor(reserveFor time.Duration) OptOptionsSetter {
	return outbox.WithReserveFor(reserveFor)
}

func WithLogger(logger Logger) OptOptionsSetter {
	return outbox.WithLogger(logger)
}

func WithTransactor(transactor Transactor) OptOptionsSetter {
	return outbox.WithTransactor(transactor)
}

func WithJobsRepo(jobsRepo JobsRepository) OptOptionsSetter {
	return outbox.WithJobsRepo(jobsRepo)
}

func WithJobsStatRepo(jobsStatRepo JobsStatRepository) OptOptionsSetter {
	return outbox.WithJobsStatRepo(jobsStatRepo)
}

func WithJobsFailedRepo(jobsFailedRepo JobsFailedRepository) OptOptionsSetter {
	return outbox.WithJobsFailedRepo(jobsFailedRepo)
}
