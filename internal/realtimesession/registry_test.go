package realtimesession //nolint:testpackage // Verifies registry tombstone and lifecycle cleanup.

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/stretchr/testify/require"
)

func TestRegistryRevokesOnlyCanonicalSession(t *testing.T) {
	var r Registry
	a := Key{SubjectID: goauth.NewSubjectID(), AuthSessionID: "session-a"}
	c := Key{SubjectID: a.SubjectID, AuthSessionID: "session-c"}
	other := Key{SubjectID: goauth.NewSubjectID(), AuthSessionID: a.AuthSessionID}
	var closed [4]atomic.Int32
	for i, key := range []Key{a, a, c, other} {
		l := r.Begin(time.Second).Bind(key, time.Second)
		require.NotNil(t, l)
		t.Cleanup(l.Release)
		require.True(t, l.Attach(func() { closed[i].Add(1) }))
	}
	r.Revoke(a)
	r.Revoke(a)
	require.EqualValues(t, 1, closed[0].Load())
	require.EqualValues(t, 1, closed[1].Load())
	require.Zero(t, closed[2].Load())
	require.Zero(t, closed[3].Load())
	fresh := r.Begin(time.Second).Bind(Key{SubjectID: a.SubjectID, AuthSessionID: "new-login"}, time.Second)
	require.NotNil(t, fresh)
	fresh.Release()
}

func TestRegistryFencesAuthenticationAndLateUpgrade(t *testing.T) {
	const afterAttach = "after-attach"
	const beforeBind = "before-bind"
	for _, phase := range []string{beforeBind, "before-attach", afterAttach} {
		t.Run(phase, func(t *testing.T) {
			var r Registry
			key := Key{SubjectID: goauth.NewSubjectID(), AuthSessionID: "a"}
			a := r.Begin(time.Second)
			unaffected := r.Begin(time.Second)
			var l *Lease
			if phase != beforeBind {
				l = a.Bind(key, time.Second)
				require.NotNil(t, l)
			}
			var closes atomic.Int32
			if phase == afterAttach {
				require.True(t, l.Attach(func() { closes.Add(1) }))
			}
			r.Revoke(key)
			if phase == beforeBind {
				require.Nil(t, a.Bind(key, time.Second))
			} else {
				require.False(t, l.Attach(func() { closes.Add(1) }))
			}
			if phase == afterAttach {
				require.EqualValues(t, 1, closes.Load())
			} else {
				require.Zero(t, closes.Load())
			}
			l = unaffected.Bind(Key{SubjectID: key.SubjectID, AuthSessionID: "c"}, time.Second)
			require.NotNil(t, l, "another session's in-flight admission must remain valid")
			l.Release()
			require.Empty(t, r.active)
			require.Empty(t, r.pending)
			require.Empty(t, r.revoked)
		})
	}
}

func TestRegistryRejectsInvalidIdentityAndCanceledAdmissions(t *testing.T) {
	var r Registry
	valid := Key{SubjectID: goauth.NewSubjectID(), AuthSessionID: "a"}
	for _, key := range []Key{
		{}, {SubjectID: valid.SubjectID}, {AuthSessionID: "a"}, {SubjectID: valid.SubjectID, AuthSessionID: " "},
	} {
		require.Nil(t, r.Begin(time.Second).Bind(key, time.Second))
		r.Revoke(key)
	}
	a := r.Begin(time.Second)
	a.Cancel()
	a.Cancel()
	require.Nil(t, a.Bind(valid, time.Second))
	var absent *Admission
	absent.Cancel()
	require.Nil(t, absent.Bind(valid, time.Second))
	require.Empty(t, r.pending)
	require.Empty(t, r.revoked)
}

func TestRegistryExpiresHungAuthenticationAndHandoff(t *testing.T) {
	var r Registry
	key := Key{SubjectID: goauth.NewSubjectID(), AuthSessionID: "a"}
	a := r.Begin(10 * time.Millisecond)
	r.Revoke(key)
	require.Eventually(t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return len(r.pending) == 0 && len(r.revoked) == 0
	}, time.Second, time.Millisecond)
	require.Nil(t, a.Bind(key, time.Second), "expired ticket must not revive after pruning")
	l := r.Begin(time.Second).Bind(key, 10*time.Millisecond)
	require.NotNil(t, l)
	require.Eventually(t, func() bool { r.mu.Lock(); defer r.mu.Unlock(); return len(r.active) == 0 }, time.Second, time.Millisecond)
	require.False(t, l.Attach(func() {}), "late callback must close its socket without subscribing")
	l.Release()
}

func TestRegistryClosesOutsideLockAndReleasesConcurrently(t *testing.T) {
	var r Registry
	key := Key{SubjectID: goauth.NewSubjectID(), AuthSessionID: "a"}
	l := r.Begin(time.Second).Bind(key, time.Second)
	require.NotNil(t, l)
	require.True(t, l.Attach(func() { l.Release(); r.Begin(time.Second).Cancel() }))
	done := make(chan struct{})
	go func() { r.Revoke(key); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("socket close ran under registry lock")
	}
	for range 100 {
		l = r.Begin(time.Second).Bind(key, time.Second)
		require.NotNil(t, l)
		var wg sync.WaitGroup
		wg.Add(3)
		go func(l *Lease) { defer wg.Done(); l.Attach(func() {}) }(l)
		go func(l *Lease) { defer wg.Done(); l.Release() }(l)
		go func() { defer wg.Done(); r.Revoke(key) }()
		wg.Wait()
	}
	require.Empty(t, r.active)
	require.Empty(t, r.pending)
	require.Empty(t, r.revoked)
}
