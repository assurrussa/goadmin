package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// A timed-out runner may still be using shared clients. The executable must
// terminate rather than close those clients and continue running.
var errRuntimeShutdownTimeout = errors.New("runtime shutdown deadline exceeded")

type runtimeResult struct {
	name string
	err  error
}

// superviseRuntime initiates a bounded drain and joins cooperative runners.
// A timeout is explicitly reported; it is never represented as a successful join.
func superviseRuntime(ctx context.Context, runOutbox func(context.Context) error, beginDrain func(),
	runServer, runNotifications func(context.Context) error,
) error {
	return superviseRuntimeWithLimits(ctx, runOutbox, beginDrain, runServer, runNotifications, 10*time.Second, 3*time.Second)
}

func superviseRuntimeWithLimits(ctx context.Context, runOutbox func(context.Context) error, beginDrain func(),
	runServer, runNotifications func(context.Context) error, drainTimeout, cancelTimeout time.Duration,
) error {
	if runOutbox == nil || beginDrain == nil || runServer == nil || runNotifications == nil || drainTimeout <= 0 || cancelTimeout <= 0 {
		return errors.New("runtime supervision requires runners and positive shutdown budgets")
	}
	if ctx.Err() != nil {
		return nil
	}
	base := context.WithoutCancel(ctx)
	workerCtx, workerStop := context.WithCancel(base)
	defer workerStop()
	serverCtx, serverStop := context.WithCancel(base)
	defer serverStop()
	notificationCtx, notificationStop := context.WithCancel(base)
	defer notificationStop()
	results := make(chan runtimeResult, 3)
	go func() { results <- runtimeResult{"outbox worker", runOutbox(workerCtx)} }()
	go func() { results <- runtimeResult{"admin server", runServer(serverCtx)} }()
	go func() { results <- runtimeResult{"auth notification worker", runNotifications(notificationCtx)} }()
	remaining := 3
	var result error
	select {
	case stopped := <-results:
		remaining--
		if ctx.Err() == nil {
			if stopped.err == nil {
				result = fmt.Errorf("%s stopped unexpectedly", stopped.name)
			} else {
				result = fmt.Errorf("%s stopped: %w", stopped.name, stopped.err)
			}
		} else if err := withoutExpectedCancellation(stopped.err); err != nil {
			result = fmt.Errorf("%s shutdown: %w", stopped.name, err)
		}
	case <-ctx.Done():
	}
	beginDrain()
	serverStop()
	notificationStop()
	timer := time.NewTimer(drainTimeout)
	defer timer.Stop()
	forced := false
	for remaining > 0 {
		select {
		case stopped := <-results:
			remaining--
			if err := withoutExpectedCancellation(stopped.err); err != nil {
				result = errors.Join(result, fmt.Errorf("%s shutdown: %w", stopped.name, err))
			}
		case <-timer.C:
			if forced {
				return errors.Join(result, fmt.Errorf("%w: %d runner(s) did not join", errRuntimeShutdownTimeout, remaining))
			}
			forced = true
			workerStop()
			timer.Reset(cancelTimeout)
		}
	}
	return result
}

// Do not discard a real failure merely because errors.Join also contains
// context.Canceled. Unwrap each branch before classifying expected cancellation.
func withoutExpectedCancellation(err error) error {
	if err == nil || err == context.Canceled {
		return nil
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var result error
		for _, cause := range joined.Unwrap() {
			result = errors.Join(result, withoutExpectedCancellation(cause))
		}
		return result
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok && wrapped.Unwrap() != nil {
		if withoutExpectedCancellation(wrapped.Unwrap()) == nil {
			return nil
		}
	}
	return err
}
