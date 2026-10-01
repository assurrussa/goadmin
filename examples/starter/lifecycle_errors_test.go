package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestShutdownPreservesAllRunnerErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	one, two, three := errors.New("outbox failed"), errors.New("server failed"), errors.New("notification failed")
	ready := make(chan struct{}, 3)
	drain := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- superviseRuntimeWithLimits(ctx,
			func(context.Context) error { ready <- struct{}{}; <-drain; return one },
			func() { close(drain) },
			func(ctx context.Context) error { ready <- struct{}{}; <-ctx.Done(); return errors.Join(ctx.Err(), two) },
			func(ctx context.Context) error { ready <- struct{}{}; <-ctx.Done(); return three },
			time.Second, time.Second)
	}()
	for i := 0; i < 3; i++ {
		select {
		case <-ready:
		case <-time.After(time.Second):
			t.Fatal("runner did not start")
		}
	}
	cancel()
	select {
	case err := <-done:
		for _, want := range []error{one, two, three} {
			if !errors.Is(err, want) {
				t.Fatalf("lost %v in %v", want, err)
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("supervisor did not join")
	}
}

func TestShutdownTimeoutIsExplicitAndDoesNotPretendJoin(t *testing.T) {
	ctx := context.Background()
	blocked := make(chan struct{})
	joined := make(chan struct{})
	defer close(blocked)
	err := superviseRuntimeWithLimits(ctx,
		func(context.Context) error { <-blocked; close(joined); return nil },
		func() {},
		func(context.Context) error { return errors.New("listener stopped") },
		func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() },
		10*time.Millisecond, 10*time.Millisecond)
	if !errors.Is(err, errRuntimeShutdownTimeout) {
		t.Fatalf("timeout not reported: %v", err)
	}
	select {
	case <-joined:
		t.Fatal("test runner unexpectedly finished")
	default:
	}
}

func TestCancellationDoesNotHideJoinedFailure(t *testing.T) {
	failure := errors.New("durable write failed")
	if err := withoutExpectedCancellation(fmt.Errorf("worker: %w", context.Canceled)); err != nil {
		t.Fatal(err)
	}
	err := withoutExpectedCancellation(fmt.Errorf("worker: %w", errors.Join(context.Canceled, failure)))
	if !errors.Is(err, failure) {
		t.Fatalf("lost joined error: %v", err)
	}
}
