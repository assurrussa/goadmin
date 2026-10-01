//go:build integration

package adminnotificationrepo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	logger "github.com/assurrussa/gologger"
	"github.com/assurrussa/gonotify/transport"
	coreoutbox "github.com/assurrussa/outbox/outbox"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/host"
	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminnotificationrepo"
	notificationsjob "github.com/assurrussa/goadmin/outbox/notifications"
	"github.com/assurrussa/goadmin/tests"
)

func TestCreateReplayKeepsOneInboxRowAndReadState(t *testing.T) {
	db, _, cleanup := tests.PrepareDB(t.Context(), t, "NotificationReplay")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cleanup(ctx)
	})
	repo := adminnotificationrepo.Must(adminnotificationrepo.NewOptions(db, host.NewTxManager(db)))
	var adminID int64
	engine, ok := db.DB().(outbox.StoragePgsqlDBPgxEnginePool)
	require.True(t, ok)
	require.NoError(t, engine.Pool().QueryRow(t.Context(), `WITH subject AS (INSERT INTO auth_subjects (id,status,created_at,updated_at) VALUES ($1,'active',now(),now()) RETURNING id) INSERT INTO administrations (subject_id,uuid) SELECT id,$2 FROM subject RETURNING id`, uuid.NewString(), uuid.NewString()).Scan(&adminID))
	p := adminnotificationrepo.CreateParams{AdminID: adminID, DispatchKey: "operation-123", Title: "Test", Message: "Message", Level: "info"}
	id, err := repo.Create(t.Context(), p)
	require.NoError(t, err)
	require.NoError(t, repo.MarkRead(t.Context(), adminID, id))
	replayID, err := repo.Create(t.Context(), p)
	require.NoError(t, err)
	require.Equal(t, id, replayID)
	list, err := repo.ListByAdmin(t.Context(), adminnotificationrepo.ListParams{AdminID: adminID, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 1, list.Total)
	require.Zero(t, list.UnreadCount)
	p.DispatchKey = "operation-456"
	secondID, err := repo.Create(t.Context(), p)
	require.NoError(t, err)
	require.NotEqual(t, id, secondID)
}

type captureNotificationJob struct {
	*notificationsjob.Job
	handled chan context.Context
}

func (j captureNotificationJob) Handle(ctx context.Context, payload string) error {
	if err := j.Job.Handle(ctx, payload); err != nil {
		return err
	}
	j.handled <- context.WithoutCancel(ctx)
	return nil
}

func TestLegacyInboxJobUsesStableOutboxIdentity(t *testing.T) {
	db, _, cleanup := tests.PrepareDB(t.Context(), t, "LegacyNotificationReplay")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cleanup(ctx)
	})
	tx := host.NewTxManager(db)
	repo := adminnotificationrepo.Must(adminnotificationrepo.NewOptions(db, tx))
	var adminID int64
	require.NoError(t, db.DB().Pool().QueryRow(t.Context(), `WITH subject AS (
 INSERT INTO auth_subjects (id,status,created_at,updated_at) VALUES ($1,'active',now(),now()) RETURNING id
 ) INSERT INTO administrations (subject_id,uuid) SELECT id,$2 FROM subject RETURNING id`, uuid.NewString(), uuid.NewString()).Scan(&adminID))
	job := notificationsjob.Must(notificationsjob.NewOptions(repo, unusedTransport{}, logger.Discard()))
	handled := make(chan context.Context, 1)
	queue, err := host.NewOutboxRuntime(tx, db, logger.Discard(), host.OutboxConfig{Workers: 1})
	require.NoError(t, err)
	require.NoError(t, queue.RegisterJob(captureNotificationJob{Job: job, handled: handled}))
	body, err := notificationsjob.MarshalPayload(notificationsjob.Payload{Title: "Legacy", Message: "Message", Recipients: []notificationsjob.Recipient{{AdminID: adminID}}})
	require.NoError(t, err)
	jobID, err := queue.Put(t.Context(), job.Name(), body, time.Now())
	require.NoError(t, err)
	runCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- queue.Run(runCtx) }()
	var attemptCtx context.Context
	select {
	case attemptCtx = <-handled:
	case <-time.After(10 * time.Second):
		cancel()
		<-done
		t.Fatal("job was not handled")
	}
	cancel()
	require.NoError(t, <-done)
	require.Equal(t, jobID, coreoutbox.JobIDFromContext(attemptCtx))
	list, err := repo.ListByAdmin(t.Context(), adminnotificationrepo.ListParams{AdminID: adminID, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 1, list.Total)
	require.NoError(t, repo.MarkRead(t.Context(), adminID, list.Notifications[0].ID))
	require.NoError(t, job.Handle(attemptCtx, body))
	list, err = repo.ListByAdmin(t.Context(), adminnotificationrepo.ListParams{AdminID: adminID, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 1, list.Total)
	require.Zero(t, list.UnreadCount)
}

type unusedTransport struct{}

func (unusedTransport) Submit(context.Context, transport.Request) (transport.Receipt, error) {
	return transport.Receipt{}, errors.New("inbox-only job must not submit")
}
