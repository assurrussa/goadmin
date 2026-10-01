package operations //nolint:testpackage // required

import (
	"context"
	"testing"
	"time"
)

func TestNewDetachedOperationContext_IsBounded(t *testing.T) {
	t.Parallel()

	ctx, cancel := newDetachedOperationContext()
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected context deadline")
	}
	if diff := deadline.Sub(time.Now().Add(operationTimeout)); diff < -time.Second || diff > time.Second {
		t.Fatalf("unexpected deadline diff: %s", diff)
	}

	select {
	case <-ctx.Done():
		t.Fatal("operation context should remain active before explicit cancel")
	default:
	}

	cancel()
	if ctx.Err() != context.Canceled {
		t.Fatalf("expected canceled context, got %v", ctx.Err())
	}
}
