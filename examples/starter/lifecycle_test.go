package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSupervisionStopsAndJoinsOnNotificationFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	workerDone := make(chan struct{})
	serverDone := make(chan struct{})
	drain := make(chan struct{})
	failure := errors.New("notification queue unavailable")
	err := superviseRuntime(ctx, func(context.Context) error {
		defer close(workerDone)
		<-drain
		return nil
	}, func() { close(drain) }, func(ctx context.Context) error {
		defer close(serverDone)
		<-ctx.Done()
		return ctx.Err()
	}, func(context.Context) error { return failure })
	if err == nil || !strings.Contains(err.Error(), "auth notification worker stopped: notification queue unavailable") {
		t.Fatalf("unexpected error %v", err)
	}
	select {
	case <-workerDone:
	default:
		t.Fatal("outbox was not joined")
	}
	select {
	case <-serverDone:
	default:
		t.Fatal("server was not joined")
	}
}

func TestSupervisionCancelsAndJoinsNotificationsOnShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	joined := make(chan struct{})
	drain := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- superviseRuntime(ctx, func(context.Context) error { <-drain; return nil }, func() { close(drain) }, func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }, func(ctx context.Context) error { close(ready); defer close(joined); <-ctx.Done(); return ctx.Err() })
	}()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("notification worker did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("supervision did not stop")
	}
	select {
	case <-joined:
	default:
		t.Fatal("notification worker was not joined")
	}
}

func TestSupervisionStopsNotificationsOnServerFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	joined := make(chan struct{})
	drain := make(chan struct{})
	err := superviseRuntime(ctx, func(context.Context) error { <-drain; return nil }, func() { close(drain) }, func(context.Context) error { return errors.New("listener failed") }, func(ctx context.Context) error { defer close(joined); <-ctx.Done(); return ctx.Err() })
	if err == nil || !strings.Contains(err.Error(), "admin server stopped: listener failed") {
		t.Fatalf("unexpected error %v", err)
	}
	select {
	case <-joined:
	default:
		t.Fatal("notifications were not joined")
	}
}

func TestSupervisionStopsNotificationsOnOutboxFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	joined := make(chan struct{})
	serverJoined := make(chan struct{})
	err := superviseRuntime(ctx, func(context.Context) error { return errors.New("outbox queue unavailable") }, func() {}, func(ctx context.Context) error { defer close(serverJoined); <-ctx.Done(); return ctx.Err() }, func(ctx context.Context) error { defer close(joined); <-ctx.Done(); return ctx.Err() })
	if err == nil || !strings.Contains(err.Error(), "outbox worker stopped: outbox queue unavailable") {
		t.Fatalf("unexpected error %v", err)
	}
	select {
	case <-joined:
	default:
		t.Fatal("notifications were not joined")
	}
	select {
	case <-serverJoined:
	default:
		t.Fatal("server was not joined")
	}
}
