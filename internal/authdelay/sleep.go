package authdelay

import (
	"context"
	"time"
)

// RunSleeper preserves the minimum auth response delay, unless the request ends.
func RunSleeper(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
