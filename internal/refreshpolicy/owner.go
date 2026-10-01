// Package refreshpolicy defines the non-retryable boundary of browser refresh.
package refreshpolicy

import (
	"strconv"
	"strings"
	"time"
)

// The refresh operation has a 10s budget plus 2s to record its outcome. A claim
// older than this bound requires login; it is NEVER released for another refresh.
const MaxOwnerAge = 30 * time.Second

func Owner(kind, unique string, now time.Time) string {
	return kind + ":" + strconv.FormatInt(now.UnixMilli(), 10) + ":" + unique
}

// RequiresLogin fences an old browser ID immediately during rotation and
// unresolved/crashed refreshers after the operation budget. Old-format claims
// are deliberately not recovered during a coordinated deployment.
func RequiresLogin(owner string, now time.Time) bool {
	if owner == "" {
		return false
	}
	parts := strings.SplitN(owner, ":", 3)
	if len(parts) != 3 || parts[0] != "refresh" || parts[2] == "" {
		return true
	}
	millis, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return true
	}
	age := now.Sub(time.UnixMilli(millis))
	return age > MaxOwnerAge || age < -10*time.Second
}

func Backoff(attempt int) time.Duration {
	delay := 25 * time.Millisecond
	for i := 0; i < attempt && delay < 250*time.Millisecond; i++ {
		delay *= 2
	}
	if delay > 250*time.Millisecond {
		return 250 * time.Millisecond
	}
	return delay
}
