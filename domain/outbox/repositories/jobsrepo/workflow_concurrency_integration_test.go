//go:build integration

package jobsrepo_test

import (
	"context"
	"sync"
	"testing"
	"time"

	workerqueue "github.com/assurrussa/outbox/outbox"
	workertypes "github.com/assurrussa/outbox/shared/types"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/domain/outbox/repositories/jobsrepo"
	"github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/tests"
)

type leaseFixture struct {
	db         outbox.StoragePgsqlClient
	tx         outbox.StoragePgsqlTxManager
	admin      *jobsrepo.Repo
	worker     *outbox.PgsqlJobsRepo
	id         outbox.JobID
	capability workerqueue.JobCapability
	token      workertypes.LeaseToken
}

func newLeaseFixture(t *testing.T) *leaseFixture {
	t.Helper()
	db, _, cleanup := tests.PrepareDB(t.Context(), t, "QueueClaimDelete")
	t.Cleanup(func() { cleanup(context.Background()) })
	worker := outbox.NewPgsqlJobsRepo(db)
	capability := workerqueue.JobCapability{Name: "workflow.claim", SchemaVersion: 2}
	id, err := worker.CreateJobVersioned(t.Context(), capability.Name, capability.SchemaVersion,
		`{"safe":true}`, time.Now().Add(-time.Minute))
	require.NoError(t, err)
	return &leaseFixture{
		db: db, tx: outbox.PgsqlTrxNew(db.DB()), admin: jobsrepo.Must(jobsrepo.NewOptions(db, worker)),
		worker: worker, id: id, capability: capability, token: workertypes.NewLeaseToken(),
	}
}

func (f *leaseFixture) claim(ctx context.Context) ([]outbox.JobModel, error) {
	now := time.Now()
	return f.worker.FindAndReserveJobsForCapabilities(ctx, now, now.Add(time.Minute), f.token,
		[]workerqueue.JobCapability{f.capability}, 1)
}

func workflowWorkerFirst(t *testing.T) {
	t.Helper()
	f := newLeaseFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
	defer cancel()
	ready, release := make(chan int, 1), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	workerDone := make(chan error, 1)
	go func() {
		workerDone <- f.tx.RunInTx(ctx, func(txCtx context.Context) error {
			jobs, err := f.claim(txCtx)
			if err != nil {
				return err
			}
			if len(jobs) != 1 || jobs[0].ID != f.id {
				return outbox.ErrNoJobs
			}
			var pid int
			if err = f.db.DB().ScanOne(txCtx, "test.claim.pid", &pid, "SELECT pg_backend_pid()"); err != nil {
				return err
			}
			ready <- pid
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	var blocker int
	select {
	case blocker = <-ready:
	case <-ctx.Done():
		t.Fatal("actual worker repository did not reserve the job")
	}
	type deleteResult struct {
		affected int64
		err      error
	}
	deleteDone := make(chan deleteResult, 1)
	go func() {
		count, err := f.admin.DeleteJob(ctx, f.id)
		deleteDone <- deleteResult{affected: count, err: err}
	}()
	waiter := 0
	for waiter == 0 {
		require.NoError(t, f.db.DB().ScanOne(ctx, "test.delete.blocked", &waiter, `SELECT COALESCE((SELECT pid FROM pg_stat_activity
    WHERE $1=ANY(pg_blocking_pids(pid)) AND wait_event_type='Lock' AND query ILIKE '%delete%jobs%' LIMIT 1),0)`, blocker))
		select {
		case <-ctx.Done():
			t.Fatal("actual DeleteJob never entered the PostgreSQL lock wait")
		default:
		}
	}
	t.Logf("PostgreSQL confirmed DeleteJob waiter pid=%d blocked by reserve transaction pid=%d", waiter, blocker)
	releaseOnce.Do(func() { close(release) })
	require.NoError(t, <-workerDone)
	deleted := <-deleteDone
	require.NoError(t, deleted.err)
	require.Zero(t, deleted.affected)
	job, err := f.worker.GetByID(ctx, f.id)
	require.NoError(t, err)
	require.True(t, job.ReservedAt.Valid)
	require.Equal(t, f.token, job.LeaseToken)
	require.Equal(t, 1, job.Attempts)
}

func workflowDeleteFirst(t *testing.T) {
	t.Helper()
	f := newLeaseFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
	defer cancel()
	ready, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	deleteDone := make(chan error, 1)
	go func() {
		deleteDone <- f.tx.RunInTx(ctx, func(txCtx context.Context) error {
			count, err := f.admin.DeleteJob(txCtx, f.id)
			if err != nil {
				return err
			}
			if count != 1 {
				return outbox.ErrNoJobs
			}
			close(ready)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal("DeleteJob did not delete within its uncommitted native transaction")
	}
	// This is an actual completed SKIP LOCKED claim while the DELETE transaction
	// remains open, not a ready signal emitted before a goroutine attempts SQL.
	jobs, err := f.claim(ctx)
	require.ErrorIs(t, err, outbox.ErrNoJobs)
	require.Empty(t, jobs)
	var visible int
	require.NoError(t, f.db.DB().ScanOne(ctx, "test.delete.visible", &visible, "SELECT count(*) FROM jobs WHERE id=$1", f.id))
	require.Equal(t, 1, visible, "the completed worker claim overlaps a still-uncommitted DELETE")
	releaseOnce.Do(func() { close(release) })
	require.NoError(t, <-deleteDone)
	require.NoError(t, f.db.DB().ScanOne(ctx, "test.delete.committed", &visible, "SELECT count(*) FROM jobs WHERE id=$1", f.id))
	require.Zero(t, visible)
}

func TestWorkflowWorkerClaimVersusDeleteJobBothOrders(t *testing.T) {
	t.Run("worker reserves first and delete rechecks live lease after lock wait", workflowWorkerFirst)
	t.Run("delete locks first and real worker skips the uncommitted delete", workflowDeleteFirst)
}
