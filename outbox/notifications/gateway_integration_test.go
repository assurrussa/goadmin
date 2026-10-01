//go:build integration

package notificationsjob_test

import (
	"context"
	"os"
	"testing"

	logger "github.com/assurrussa/gologger"
	"github.com/assurrussa/gonotify/transport"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/host"
	notificationsjob "github.com/assurrussa/goadmin/outbox/notifications"
)

// Run explicitly against an isolated NotifyHub project with a dry_run provider.
// It exercises the real gateway's durable acceptance and idempotency contract.
func TestNotifyHubGateway(t *testing.T) {
	baseURL := os.Getenv("GOADMIN_NOTIFYHUB_TEST_URL")
	if baseURL == "" {
		t.Skip("isolated NotifyHub gateway was not configured")
	}
	gateway, err := host.NewNotificationManager(host.NotificationConfig{BaseURL: baseURL, ProjectKey: os.Getenv("GOADMIN_NOTIFYHUB_TEST_KEY")})
	require.NoError(t, err)
	p := deliveryPayload()
	p.DispatchID = uuid.NewString()
	repo := &repoStub{}
	job := notificationsjob.Must(notificationsjob.NewOptions(repo, gateway, logger.Discard()))
	raw, err := notificationsjob.MarshalPayload(p)
	require.NoError(t, err)
	require.NoError(t, job.Handle(t.Context(), raw))
	requests, err := notificationsjob.DeliveryRequests(p)
	require.NoError(t, err)
	first, err := gateway.Submit(t.Context(), requests[0])
	require.NoError(t, err)
	require.True(t, first.Duplicate)
	require.NotEmpty(t, first.ID)
	require.NoError(t, job.Handle(t.Context(), raw))
	require.Len(t, repo.items, 1)
	second, err := gateway.Submit(t.Context(), requests[0])
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	require.True(t, second.Duplicate)
	changed := requests[0]
	email := *changed.Email
	email.Text = "changed content"
	changed.Email = &email
	_, err = gateway.Submit(context.Background(), changed)
	require.ErrorIs(t, err, transport.ErrIdempotencyConflict)
}
