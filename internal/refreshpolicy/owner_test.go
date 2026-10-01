package refreshpolicy_test

import (
	"testing"
	"time"

	"github.com/assurrussa/goadmin/internal/refreshpolicy"
)

func TestOwnerFencesExpiredAndRotationClaims(t *testing.T) {
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, owner string
		login       bool
	}{
		{"free", "", false},
		{"active", refreshpolicy.Owner("refresh", "unique", now), false},
		{"bounded skew", refreshpolicy.Owner("refresh", "unique", now.Add(time.Second)), false},
		{"expired", refreshpolicy.Owner("refresh", "unique", now.Add(-refreshpolicy.MaxOwnerAge-time.Millisecond)), true},
		{"rotation", refreshpolicy.Owner("rotate", "unique", now), true},
		{"legacy", "old-unresolved-owner", true},
		{"bad time", "refresh:bad:unique", true},
		{"future", refreshpolicy.Owner("refresh", "unique", now.Add(time.Minute)), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := refreshpolicy.RequiresLogin(tc.owner, now); got != tc.login {
				t.Fatalf("got %v", got)
			}
		})
	}
}

func TestBackoffIsBounded(t *testing.T) {
	var previous time.Duration
	for i := 0; i < 100; i++ {
		delay := refreshpolicy.Backoff(i)
		if delay < 25*time.Millisecond || delay > 250*time.Millisecond || delay < previous {
			t.Fatalf("backoff(%d)=%s", i, delay)
		}
		previous = delay
	}
}
