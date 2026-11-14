package authsnapshot //nolint:testpackage // white-box generation, clock and loader publication tests

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/rbac"
	"github.com/stretchr/testify/require"
)

var (
	adminID    = goauth.NewSubjectID()
	customerID = goauth.NewSubjectID()
	unknownID  = goauth.NewSubjectID()
)

func testData() Data {
	return Data{Members: []goauth.SubjectID{adminID}, RBAC: rbac.Snapshot{
		Roles:           []rbac.Role{{ID: 1, Slug: "editor"}},
		Permissions:     []rbac.Permission{{ID: 2, Key: "page:read"}},
		RolePermissions: []rbac.RolePermission{{RoleID: 1, PermissionID: 2}},
		SubjectRoles:    []rbac.SubjectRole{{SubjectID: adminID, RoleID: 1}, {SubjectID: customerID, RoleID: 1}},
	}}
}

func testConfig() Config {
	return Config{RefreshInterval: time.Hour, MaxAge: 2 * time.Hour, LoadTimeout: time.Second}
}

func TestSnapshotMembershipAndPermissionsAreOneView(t *testing.T) {
	var loads atomic.Int64
	c, err := New(t.Context(), func(context.Context) (Data, error) { loads.Add(1); return testData(), nil }, testConfig())
	require.NoError(t, err)
	defer c.Close()
	for range 1000 {
		v, err := c.View(t.Context())
		require.NoError(t, err)
		require.True(t, v.Member(adminID))
		require.True(t, v.Allows(adminID, "page:read"))
		require.False(t, v.Member(customerID))
		require.False(t, v.Allows(customerID, "page:read"))
		roles := v.Roles(adminID)
		roles[0].Slug = "mutated"
		require.Equal(t, "editor", v.Roles(adminID)[0].Slug)
		allowed, found, err := c.HasPermission(t.Context(), unknownID, "page:read")
		require.NoError(t, err)
		require.False(t, allowed)
		require.True(t, found)
	}
	require.EqualValues(t, 1, loads.Load(), "read decisions must not call the database loader")
}

func TestFailedInitialLoadDoesNotCreateCache(t *testing.T) {
	c, err := New(t.Context(), func(context.Context) (Data, error) { return Data{}, errors.New("offline") }, testConfig())
	require.Error(t, err)
	require.Nil(t, c)
}

func TestExpiredSnapshotFailsClosedAndRefreshRecovers(t *testing.T) {
	var fail atomic.Bool
	c, err := New(t.Context(), func(context.Context) (Data, error) {
		if fail.Load() {
			return Data{}, errors.New("offline")
		}
		return testData(), nil
	}, testConfig())
	require.NoError(t, err)
	defer c.Close()
	fail.Store(true)
	require.Error(t, c.data.Refresh(t.Context()))
	_, err = c.View(t.Context())
	require.NoError(t, err, "last complete snapshot remains valid within configured bound")
	old, _ := c.data.Get(0)
	c.now = func() time.Time { return old.loadedAt.Add(c.maxAge) }
	_, err = c.View(t.Context())
	require.ErrorIs(t, err, ErrUnavailable)
	fail.Store(false)
	require.NoError(t, c.data.Refresh(t.Context()))
	_, err = c.View(t.Context())
	require.NoError(t, err)
	c.Close()
	_, err = c.View(t.Context())
	require.ErrorIs(t, err, ErrUnavailable)
}

func TestInvalidationFencesLoadAlreadyInFlight(t *testing.T) {
	var calls atomic.Int64
	started, release := make(chan struct{}), make(chan struct{})
	thirdStarted, thirdRelease := make(chan struct{}), make(chan struct{})
	c, err := New(t.Context(), func(ctx context.Context) (Data, error) {
		switch calls.Add(1) {
		case 2:
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return Data{}, ctx.Err()
			}
		case 3:
			close(thirdStarted)
			select {
			case <-thirdRelease:
			case <-ctx.Done():
				return Data{}, ctx.Err()
			}
			return Data{}, nil
		}
		return testData(), nil
	}, testConfig())
	require.NoError(t, err)
	defer c.Close()
	done := make(chan error, 1)
	go func() { done <- c.data.Refresh(t.Context()) }()
	<-started
	require.NoError(t, c.Invalidate(t.Context()))
	_, err = c.View(t.Context())
	require.ErrorIs(t, err, ErrUnavailable)
	close(release)
	require.NoError(t, <-done)
	<-thirdStarted
	_, err = c.View(t.Context())
	require.ErrorIs(t, err, ErrUnavailable, "old generation must not resurrect grants")
	close(thirdRelease)
	require.Eventually(t, func() bool {
		v, e := c.View(t.Context())
		return e == nil && !v.Member(adminID)
	}, time.Second, time.Millisecond)
}

func TestConcurrentReadersAndRefresh(t *testing.T) {
	c, err := New(t.Context(), func(context.Context) (Data, error) { return testData(), nil }, testConfig())
	require.NoError(t, err)
	defer c.Close()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 1000 {
				v, e := c.View(t.Context())
				if e == nil && !v.Allows(adminID, "page:read") {
					t.Error("torn authorization view")
				}
			}
		})
	}
	for range 20 {
		require.NoError(t, c.data.Refresh(t.Context()))
	}
	wg.Wait()
}

func TestConfigurationAndCancellation(t *testing.T) {
	loader := func(context.Context) (Data, error) { return testData(), nil }
	_, err := New(nil, loader, testConfig()) //nolint:staticcheck // exercise explicit nil-context rejection
	require.Error(t, err)
	_, err = New(t.Context(), nil, testConfig())
	require.Error(t, err)
	cfg := testConfig()
	cfg.MaxAge = cfg.RefreshInterval
	_, err = New(t.Context(), loader, cfg)
	require.Error(t, err)
	c, err := New(t.Context(), loader, testConfig())
	require.NoError(t, err)
	defer c.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = c.View(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, c.Invalidate(ctx), context.Canceled)
}

func TestPinnedViewDoesNotMixPolicyAcrossRefresh(t *testing.T) {
	var changed atomic.Bool
	c, err := New(t.Context(), func(context.Context) (Data, error) {
		if changed.Load() {
			return Data{}, nil
		}
		return testData(), nil
	}, testConfig())
	require.NoError(t, err)
	defer c.Close()
	before, err := c.View(t.Context())
	require.NoError(t, err)
	require.True(t, before.Member(adminID))
	changed.Store(true)
	require.NoError(t, c.data.Refresh(t.Context()))
	after, err := c.View(t.Context())
	require.NoError(t, err)
	require.False(t, after.Member(adminID))
	require.False(t, after.Allows(adminID, "page:read"))
	// A composite decision uses this same before value for BOTH checks. It never
	// takes membership from before and permissions from after. Handed-out views
	// are immutable snapshots, not revocable handles, and are request-bound.
	require.True(t, before.Member(adminID))
	require.True(t, before.Allows(adminID, "page:read"))
}

func TestPublishedDataDoesNotAliasLoaderSlices(t *testing.T) {
	source := testData()
	c, err := New(t.Context(), func(context.Context) (Data, error) { return source, nil }, testConfig())
	require.NoError(t, err)
	defer c.Close()
	source.Members[0] = customerID
	source.RBAC.Roles[0].Slug = "changed"
	source.RBAC.Permissions[0].Key = "page:write"
	source.RBAC.RolePermissions[0].RoleID = 9
	source.RBAC.SubjectRoles[0].SubjectID = customerID
	v, err := c.View(t.Context())
	require.NoError(t, err)
	require.True(t, v.Member(adminID))
	require.False(t, v.Member(customerID))
	require.True(t, v.Allows(adminID, "page:read"))
	require.False(t, v.Allows(adminID, "page:write"))
	require.Equal(t, "editor", v.Roles(adminID)[0].Slug)
}

func BenchmarkSnapshotRead(b *testing.B) {
	data := testData()
	c, err := New(b.Context(), func(context.Context) (Data, error) { return data, nil }, testConfig())
	if err != nil {
		b.Fatal(err)
	}
	defer c.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		v, err := c.View(b.Context())
		if err != nil || !v.Allows(adminID, "page:read") {
			b.Fatal("invalid view")
		}
	}
}
