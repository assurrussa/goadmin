//go:build integration

package tests

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/stretchr/testify/require"
)

// AuthRuntimeConfig returns isolated keys for the PostgreSQL-owned auth runtime.
// Pair it with AuthNotificationSender so PostgreSQL assembles its durable queue.
func AuthRuntimeConfig(t *testing.T) goauth.Config {
	t.Helper()

	return goauth.Config{
		Signing: goauth.SigningConfig{
			Issuer: "https://auth.goadmin.test", Audience: "goadmin-integration",
			Keys: testKeyRing(t, "jwt", 1),
		},
		TokenHMACKeys:  testKeyRing(t, "token", 2),
		OutboxAEADKeys: testKeyRing(t, "outbox", 3),
		URLBuilder: goauth.URLBuilderFunc(func(_ context.Context, token string) (string, error) {
			return "https://admin.goadmin.test/reset-password?token=" + url.QueryEscape(token), nil
		}),
		ResetResponseFloor: time.Nanosecond,
	}
}

func testKeyRing(t *testing.T, purpose string, fill byte) goauth.KeyRing {
	t.Helper()
	ring, err := goauth.NewKeyRing(
		purpose+"-v1",
		goauth.Key{ID: purpose + "-v1", Material: bytes.Repeat([]byte{fill}, 32)},
	)
	require.NoError(t, err)

	return ring
}

// AuthNotificationSender enables managed delivery wiring without sending mail.
// Repository tests never start its worker; unexpected delivery fails explicitly.
func AuthNotificationSender() goauth.NotificationSender {
	return goauth.NotificationSenderFunc(func(context.Context, goauth.NotificationDelivery) error {
		return errors.New("repository test must not send notifications")
	})
}
