package adminnotifications_test

import (
	"context"
	"testing"
	"time"

	logger "github.com/assurrussa/gologger"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/infrastructure/notify"
	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/models"
	notificationsjob "github.com/assurrussa/goadmin/outbox/notifications"
	"github.com/assurrussa/goadmin/services/adminnotifications"
)

type outboxStub struct {
	lastName  string
	lastBody  string
	shouldErr error
}

func (s *outboxStub) Put(_ context.Context, name, payload string, _ time.Time) (outbox.JobID, error) {
	s.lastName = name
	s.lastBody = payload
	if s.shouldErr != nil {
		return outbox.JobIDNil, s.shouldErr
	}
	return outbox.NewJobID(), nil
}

type adminRepoStub struct {
	data map[int64]models.Admin
}

func (r *adminRepoStub) GetByID(_ context.Context, id int64) (models.Admin, error) {
	if admin, ok := r.data[id]; ok {
		return admin, nil
	}
	return models.Admin{}, nil
}

func TestServiceEnqueue_Success(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	outbox := &outboxStub{}
	repo := &adminRepoStub{
		data: map[int64]models.Admin{
			1: {
				ID:       1,
				Name:     "Admin",
				LastName: "One",
				Email:    "admin@example.com",
			},
		},
	}
	service := adminnotifications.NewService(outbox, repo, logger.Discard())

	jobID, err := service.Enqueue(ctx, adminnotifications.Request{
		AdminIDs:  []int64{1},
		Title:     "Новая роль",
		Message:   "Вам назначили новую роль",
		Channels:  []notify.NotificationChannel{notify.ChannelEmail},
		EmailFrom: "noreply@example.com",
	})

	require.NoError(t, err)
	require.False(t, jobID.IsZero())
	require.Equal(t, notificationsjob.JobName, outbox.lastName)

	payload, err := notificationsjob.UnmarshalPayload(outbox.lastBody)
	require.NoError(t, err)
	require.Equal(t, "Новая роль", payload.Title)
	require.NotEmpty(t, payload.DispatchID)
	require.Equal(t, "noreply@example.com", payload.EmailFrom)
	require.Len(t, payload.Recipients, 1)
	require.Equal(t, int64(1), payload.Recipients[0].AdminID)
	require.Equal(t, []notify.NotificationChannel{notify.ChannelEmail}, payload.Channels)
}

func TestServiceEnqueue_ValidationError(t *testing.T) {
	t.Parallel()

	service := adminnotifications.NewService(&outboxStub{}, &adminRepoStub{}, logger.Discard())

	_, err := service.Enqueue(context.Background(), adminnotifications.Request{
		AdminIDs: nil,
		Title:    "Hi",
		Message:  "Body",
	})
	require.Error(t, err)

	_, err = service.Enqueue(context.Background(), adminnotifications.Request{
		AdminIDs: []int64{1},
		Title:    "",
		Message:  "Body",
	})
	require.Error(t, err)

	_, err = service.Enqueue(context.Background(), adminnotifications.Request{
		AdminIDs: []int64{1},
		Title:    "Hi",
		Message:  "",
	})
	require.Error(t, err)
}
