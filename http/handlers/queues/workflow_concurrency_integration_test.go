//go:build integration

package queues //nolint:testpackage // Exercises the actual retry command using native repositories and transactions.

import (
	"context"
	"sync"
	"testing"
	"time"

	logger "github.com/assurrussa/gologger"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/adminapp"
	"github.com/assurrussa/goadmin/adminapp/adminappt"
	"github.com/assurrussa/goadmin/domain/outbox/repositories/jobsfailedrepo"
	"github.com/assurrussa/goadmin/domain/outbox/repositories/jobsrepo"
	"github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminactionauditrepo"
	"github.com/assurrussa/goadmin/internal/admintx"
	"github.com/assurrussa/goadmin/internal/envs"
	"github.com/assurrussa/goadmin/tests"
)

type retryAuditGate struct {
	db      outbox.StoragePgsqlClient
	writer  adminactionauditrepo.Writer
	entered chan int
	release chan struct{}
}

func (g retryAuditGate) RecordAdminAction(ctx context.Context, r adminactionauditrepo.Record) error {
	if err := g.writer.RecordAdminAction(ctx, r); err != nil {
		return err
	}
	var pid int
	if err := admintx.Wrap(g.db.DB()).ScanOne(ctx, "test.retry.pid", &pid, "SELECT pg_backend_pid()"); err != nil {
		return err
	}
	g.entered <- pid
	select {
	case <-g.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestWorkflowConcurrentFailedRetryCreatesOneJobAndAudit(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
	defer cancel()
	db, _, cleanup := tests.PrepareDB(ctx, t, "QueueConcurrentRetry")
	t.Cleanup(func() { cleanup(context.Background()) })
	nativeJobs := outbox.NewPgsqlJobsRepo(db)
	nativeFailed := outbox.NewPgsqlJobsFailedRepo(db)
	id, err := nativeFailed.CreateFailedJobVersioned(ctx, outbox.NewJobID(),
		"workflow.retry", 3, `{"safe":"payload"}`, "test failure")
	require.NoError(t, err)
	gate := retryAuditGate{
		db: db, writer: adminactionauditrepo.Must(adminactionauditrepo.NewOptions(db)),
		entered: make(chan int, 1), release: make(chan struct{}),
	}
	var release sync.Once
	defer release.Do(func() { close(gate.release) })
	harness := adminappt.NewAppTest(t)
	app := adminapp.Must(adminapp.NewOptions(harness.InertiaManager, harness.MockPermissionGuard, harness.MockRoleService,
		harness.AdminService, harness.SessionStore, harness.EventStreamService, harness.MockOutbox, outbox.PgsqlTrxNew(db.DB()),
		logger.Discard(), envs.EnvProd, "https://files.example.test"))
	handler := NewHandler(app, jobsrepo.Must(jobsrepo.NewOptions(db, nativeJobs)),
		jobsfailedrepo.Must(jobsfailedrepo.NewOptions(db, nativeFailed)), gate)
	type result struct {
		related string
		err     error
	}
	first, second := make(chan result, 1), make(chan result, 1)
	record := adminactionauditrepo.Record{
		ActorAdminID: 9, Action: adminactionauditrepo.ActionQueueFailedRetry,
		TargetType: "queue_failed_job", TargetID: id.String(),
	}
	go func() {
		related, e := handler.retryFailedJob(ctx, id, record)
		first <- result{related: related, err: e}
	}()
	var blocker int
	select {
	case blocker = <-gate.entered:
	case <-ctx.Done():
		t.Fatal("first retry did not reach real audit insert")
	}
	go func() {
		related, e := handler.retryFailedJob(ctx, id, record)
		second <- result{related: related, err: e}
	}()
	var waiter int
	for waiter == 0 {
		require.NoError(t, db.DB().ScanOne(ctx, "test.retry.blocked", &waiter, `SELECT COALESCE((SELECT pid FROM pg_stat_activity
   WHERE $1=ANY(pg_blocking_pids(pid)) AND wait_event_type='Lock' AND query ILIKE '%delete%jobs_failed%' LIMIT 1),0)`, blocker))
		select {
		case <-ctx.Done():
			t.Fatal("second actual retry DELETE never waited for first transaction")
		default:
		}
	}
	t.Logf("PostgreSQL confirmed retry DELETE waiter pid=%d blocked by first retry transaction pid=%d", waiter, blocker)
	release.Do(func() { close(gate.release) })
	firstResult, secondResult := <-first, <-second
	require.NoError(t, firstResult.err)
	require.NotEmpty(t, firstResult.related)
	require.ErrorIs(t, secondResult.err, outbox.ErrNoJobs)
	require.Empty(t, secondResult.related)
	var jobs, failed, audits int
	require.NoError(t, db.DB().ScanOne(ctx, "test.retry.jobs", &jobs, "SELECT count(*) FROM jobs"))
	require.NoError(t, db.DB().ScanOne(ctx, "test.retry.failed", &failed, "SELECT count(*) FROM jobs_failed"))
	require.NoError(t, db.DB().ScanOne(ctx, "test.retry.audits", &audits, "SELECT count(*) FROM admin_action_audit"))
	require.Equal(t, 1, jobs)
	require.Zero(t, failed)
	require.Equal(t, 1, audits)
	var related string
	require.NoError(t, db.DB().ScanOne(ctx, "test.retry.related", &related, "SELECT related_target_id FROM admin_action_audit"))
	require.Equal(t, firstResult.related, related)
	newID, err := outbox.Parse[outbox.JobID](related)
	require.NoError(t, err)
	newJob, err := nativeJobs.GetByID(ctx, newID)
	require.NoError(t, err)
	require.EqualValues(t, 3, newJob.SchemaVersion)
	require.JSONEq(t, `{"safe":"payload"}`, newJob.Payload)
	require.Zero(t, newJob.Attempts)
	require.False(t, newJob.ReservedAt.Valid)
}
