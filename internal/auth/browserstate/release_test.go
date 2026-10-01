package browserstate_test

import (
	"context"
	"testing"
	"time"

	"github.com/assurrussa/goadmin/internal/auth/browserstate"
	"github.com/assurrussa/goadmin/internal/refreshpolicy"
)

func TestReleaseClaimCannotReleaseAnotherOwner(t *testing.T) {
	ctx := context.Background()
	store := browserstate.NewMemory()
	if err := store.Create(ctx, "browser", browserstate.Record{}, time.Hour); err != nil {
		t.Fatal(err)
	}
	mustClaim := func(owner string) {
		t.Helper()
		ok, err := store.Claim(ctx, "browser", 1, owner)
		if err != nil || !ok {
			t.Fatalf("claim %q: ok=%v err=%v", owner, ok, err)
		}
	}
	mustClaim("one")
	if ok, err := store.ReleaseClaim(ctx, "browser", 1, "two"); err != nil || ok {
		t.Fatalf("foreign release: ok=%v err=%v", ok, err)
	}
	if ok, err := store.ReleaseClaim(ctx, "browser", 2, "one"); err != nil || ok {
		t.Fatalf("wrong-version release: ok=%v err=%v", ok, err)
	}
	if ok, err := store.ReleaseClaim(ctx, "browser", 1, "one"); err != nil || !ok {
		t.Fatalf("owned release: ok=%v err=%v", ok, err)
	}
	mustClaim("two")
	if ok, err := store.ReleaseClaim(ctx, "browser", 1, "one"); err != nil || ok {
		t.Fatalf("late release: ok=%v err=%v", ok, err)
	}
	record, found, err := store.Load(ctx, "browser")
	if err != nil || !found || record.Owner != "two" || record.Version != 1 {
		t.Fatalf("unexpected state: %#v found=%v err=%v", record, found, err)
	}
}

func TestGuardedStoreHidesButDoesNotUnlockAbandonedState(t *testing.T) {
	for _, kind := range []string{"refresh", "rotate"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			inner := browserstate.NewMemory()
			store := browserstate.Guarded(inner)
			if err := store.Create(ctx, "browser", browserstate.Record{}, time.Hour); err != nil {
				t.Fatal(err)
			}
			at := time.Now()
			if kind == "refresh" {
				at = at.Add(-time.Minute)
			}
			owner := refreshpolicy.Owner(kind, "original", at)
			if ok, err := store.Claim(ctx, "browser", 1, owner); err != nil || !ok {
				t.Fatalf("claim: %v %v", ok, err)
			}
			if _, found, err := store.Load(ctx, "browser"); err != nil || found {
				t.Fatalf("abandoned identity visible: %v %v", found, err)
			}
			if ok, err := store.Claim(ctx, "browser", 1, "retry"); err != nil || ok {
				t.Fatalf("old refresh was made claimable: %v %v", ok, err)
			}
			if ok, err := store.Complete(ctx, "browser", 1, owner, browserstate.Record{}); err != nil || ok {
				t.Fatalf("late owner restored identity: %v %v", ok, err)
			}
			record, found, err := inner.Load(ctx, "browser")
			if err != nil || !found || record.Owner != owner {
				t.Fatalf("fence was discarded: %v %v", found, err)
			}
		})
	}
}
