//go:build integration

package tests_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/tests"
)

func TestPrepareRedisRequiresReachableService(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	address := net.JoinHostPort(tests.Config.RedisAddr, tests.Config.RedisPort)
	client, cleanup := tests.PrepareRedis(
		ctx,
		t,
		"redis-integration-gate",
		tests.WithRedisAddress(address),
	)
	t.Cleanup(func() { cleanup(context.Background()) })

	response, err := client.Ping(ctx).Result()
	require.NoError(t, err)
	require.Equal(t, "PONG", response)
}
