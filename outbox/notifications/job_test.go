package notificationsjob_test

import (
	"context"
	"errors"
	"testing"
	"time"

	logger "github.com/assurrussa/gologger"
	"github.com/assurrussa/gonotify/transport"
	coreoutbox "github.com/assurrussa/outbox/outbox"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/infrastructure/notify"
	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminnotificationrepo"
	notificationsjob "github.com/assurrussa/goadmin/outbox/notifications"
)

type repoStub struct {
	items map[string]adminnotificationrepo.CreateParams
}

func (r *repoStub) Create(_ context.Context, p adminnotificationrepo.CreateParams) (int64, error) {
	if r.items == nil {
		r.items = make(map[string]adminnotificationrepo.CreateParams)
	}
	r.items[p.DispatchKey] = p
	return int64(len(r.items)), nil
}

type transportStub struct {
	requests []transport.Request
	err      error
}

func (s *transportStub) Submit(_ context.Context, req transport.Request) (transport.Receipt, error) {
	s.requests = append(s.requests, req)
	return transport.Receipt{ID: "accepted"}, s.err
}

func deliveryPayload() notificationsjob.Payload {
	return notificationsjob.Payload{
		DispatchID: "business-operation-123", EmailFrom: "noreply@example.com",
		Title: "Test", Message: "Message", Level: notify.LevelInfo,
		Channels:   []notify.NotificationChannel{notify.ChannelEmail},
		Recipients: []notificationsjob.Recipient{{AdminID: 1, Email: "user@example.com"}},
	}
}

func TestJobHandleRetryPreservesInboxAndGatewayIdentity(t *testing.T) {
	t.Parallel()
	gateway := &transportStub{err: transport.ErrTemporarilyUnavailable}
	repo := &repoStub{}
	job := notificationsjob.Must(notificationsjob.NewOptions(repo, gateway, logger.Discard()))
	raw, err := notificationsjob.MarshalPayload(deliveryPayload())
	require.NoError(t, err)
	require.ErrorIs(t, job.Handle(t.Context(), raw), transport.ErrTemporarilyUnavailable)
	gateway.err = nil
	require.NoError(t, job.Handle(t.Context(), raw))
	require.Len(t, repo.items, 1)
	require.Len(t, gateway.requests, 2)
	require.Equal(t, gateway.requests[0], gateway.requests[1])
	require.Equal(t, "noreply@example.com", gateway.requests[0].Email.From)
	require.Equal(t, []string{"user@example.com"}, gateway.requests[0].Email.To)
	require.NotEmpty(t, gateway.requests[0].IdempotencyKey)
}

func TestJobHandlePreservesDeliveryDispositions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                       string
		err                        error
		permanent, retry, deferJob bool
	}{
		{name: "invalid", err: transport.ErrInvalidRequest, permanent: true},
		{name: "conflict", err: transport.ErrIdempotencyConflict, permanent: true},
		{name: "oversize", err: transport.ErrPayloadTooLarge, permanent: true},
		{name: "quota", err: &transport.QuotaError{RetryAfter: time.Hour}, retry: true},
		{name: "unauthorized", err: transport.ErrUnauthorized, deferJob: true},
		{name: "transient", err: transport.ErrTemporarilyUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gateway := &transportStub{err: tc.err}
			job := notificationsjob.Must(notificationsjob.NewOptions(&repoStub{}, gateway, logger.Discard()))
			raw, err := notificationsjob.MarshalPayload(deliveryPayload())
			require.NoError(t, err)
			before := time.Now()
			err = job.Handle(t.Context(), raw)
			require.ErrorIs(t, err, tc.err)
			require.Equal(t, tc.permanent, coreoutbox.IsPermanent(err))
			retryAt, retry := coreoutbox.RetryTime(err)
			require.Equal(t, tc.retry, retry)
			deferAt, deferJob := coreoutbox.DeferTime(err)
			require.Equal(t, tc.deferJob, deferJob)
			if retry {
				require.False(t, retryAt.Before(before.Add(time.Hour)))
			}
			if deferJob {
				require.True(t, deferAt.After(before))
			}
		})
	}
}

func TestJobRejectsUnsafePayloadBeforePersistence(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		change func(*notificationsjob.Payload)
	}{
		{name: "legacy external without identity", change: func(p *notificationsjob.Payload) { p.DispatchID = "" }},
		{name: "missing email sender", change: func(p *notificationsjob.Payload) { p.EmailFrom = "" }},
		{name: "unsupported channel", change: func(p *notificationsjob.Payload) { p.Channels = []notify.NotificationChannel{"push"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := deliveryPayload()
			tc.change(&p)
			repo := &repoStub{}
			gateway := &transportStub{}
			job := notificationsjob.Must(notificationsjob.NewOptions(repo, gateway, logger.Discard()))
			raw, err := notificationsjob.MarshalPayload(p)
			require.NoError(t, err)
			require.True(t, coreoutbox.IsPermanent(job.Handle(t.Context(), raw)))
			require.Empty(t, repo.items)
			require.Empty(t, gateway.requests)
		})
	}
}

func TestJobRejectsLegacyWithoutOutboxIdentity(t *testing.T) {
	t.Parallel()
	p := deliveryPayload()
	p.DispatchID = ""
	p.Channels = nil
	gateway := &transportStub{err: errors.New("must not submit")}
	repo := &repoStub{}
	job := notificationsjob.Must(notificationsjob.NewOptions(repo, gateway, logger.Discard()))
	raw, err := notificationsjob.MarshalPayload(p)
	require.NoError(t, err)
	require.True(t, coreoutbox.IsPermanent(job.Handle(t.Context(), raw)))
	require.Empty(t, repo.items)
	require.Empty(t, gateway.requests)
}

func TestDeliveryRequestsDeduplicatesAndSeparatesRecipientsAndChannels(t *testing.T) {
	t.Parallel()
	p := deliveryPayload()
	p.Channels = []notify.NotificationChannel{notify.ChannelEmail, notify.ChannelTelegram, notify.ChannelEmail}
	p.Recipients = []notificationsjob.Recipient{
		{AdminID: 1, Email: "one@example.com", TelegramChatID: 101},
		{AdminID: 2, Email: "two@example.com", TelegramChatID: 102},
	}
	requests, err := notificationsjob.DeliveryRequests(p)
	require.NoError(t, err)
	require.Len(t, requests, 4)
	keys := make(map[string]bool)
	for _, req := range requests {
		require.False(t, keys[req.IdempotencyKey])
		keys[req.IdempotencyKey] = true
	}
	require.Equal(t, "101", requests[2].Telegram.ChatID)
}
