package dashboard //nolint:testpackage // exercises the provider timeout without a slow wall-clock test.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	auth "github.com/assurrussa/goadmin/internal/auth"
)

var testActor = Actor{AdminID: 42, SubjectID: "0065c840-bb75-469b-9529-5147449ad0ed"}

func testCard(key string) Card {
	return Card{
		Key: key, Title: Text{Text: "Reports", RU: "Отчёты", EN: "Reports"},
		PermissionKey: auth.NewPermissionKey("reports", "read"),
		Links:         []Link{{Label: Text{Text: "Open"}, Href: "/reports"}},
		Load: func(context.Context, Actor) (Content, error) {
			return Content{Metrics: []Metric{{Key: "count", Label: Text{Text: "Count"}, Value: "7"}}}, nil
		},
	}
}

func TestRegistrationValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Card)
	}{
		{"empty key", func(c *Card) { c.Key = "" }},
		{"bad key", func(c *Card) { c.Key = "contains space" }},
		{"reserved", func(c *Card) { c.Key = "goadmin.custom" }},
		{"permission", func(c *Card) { c.PermissionKey = auth.PermissionKey{} }},
		{"malformed permission", func(c *Card) { c.PermissionKey.Domain = "has:colon" }},
		{"partial link permission", func(c *Card) { c.Links[0].PermissionKey.Domain = "partial" }},
		{"title", func(c *Card) { c.Title.Text = " " }},
		{"loader", func(c *Card) { c.Load = nil }},
		{"link title", func(c *Card) { c.Links[0].Label.Text = "" }},
		{"link URL", func(c *Card) { c.Links[0].Href = "https://external.test" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			card := testCard("host.reports")
			tc.mutate(&card)
			_, err := New(nil, []Card{card})
			require.Error(t, err)
		})
	}
	_, err := New(nil, []Card{testCard("duplicate"), testCard("duplicate")})
	require.ErrorContains(t, err, "duplicate")
}

func TestSafeHref(t *testing.T) {
	for _, href := range []string{"/", "/reports", "/notifications?status=unread", "/reports#latest", "/reports?q=%D1%82"} {
		require.True(t, SafeHref(href), href)
	}
	for _, href := range []string{
		"", "reports", "https://example.test", "//evil.test", "/\\evil.test", "/%2fevil.test", "/%5cevil.test",
		"/x\n", "/%00", "/bad%", "javascript:alert(1)",
	} {
		require.False(t, SafeHref(href), href)
	}
}

func TestAuthorizeBeforeProvidersAndDetachedSnapshots(t *testing.T) {
	var forbiddenCalls atomic.Int64
	denied := testCard("denied")
	denied.PermissionKey = auth.NewPermissionKey("secret", "read")
	denied.Load = func(context.Context, Actor) (Content, error) { forbiddenCalls.Add(1); return Content{}, nil }
	allowed := testCard("allowed")
	allowed.Links = append(allowed.Links,
		Link{Label: Text{Text: "Write"}, Href: "/reports/edit", PermissionKey: auth.NewPermissionKey("reports", "write")})
	registry, err := New(nil, []Card{denied, allowed})
	require.NoError(t, err)
	allowed.Links[0].Href = "/mutated"
	checks := 0
	request := registry.Authorize(testActor, func(key auth.PermissionKey) bool { checks++; return key == allowed.PermissionKey })
	snapshot := request.Descriptors()
	require.Equal(t, 3, checks)
	require.Equal(t, "42", snapshot.Owner)
	require.Len(t, snapshot.Cards, 1)
	require.Equal(t, "loading", snapshot.Cards[0].State)
	require.Empty(t, snapshot.Cards[0].Metrics)
	require.Len(t, snapshot.Cards[0].Links, 1)
	require.Equal(t, "/reports", snapshot.Cards[0].Links[0].Href)
	snapshot.Cards[0].Links[0].Href = "/changed"
	snapshot.Cards[0].Title.RU = "changed"
	fresh := request.Descriptors()
	require.Equal(t, "/reports", fresh.Cards[0].Links[0].Href)
	require.Equal(t, "Отчёты", fresh.Cards[0].Title.RU)
	loaded := request.Load(t.Context(), nil)
	require.Equal(t, "ready", loaded.Cards[0].State)
	require.EqualValues(t, 0, forbiddenCalls.Load())
	require.Equal(t, 3, checks, "authorization must finish before worker execution")
	wire, err := json.Marshal(loaded)
	require.NoError(t, err)
	require.NotContains(t, string(wire), "PermissionKey")
	require.Contains(t, string(wire), `"title":{"text":"Reports","ru":"Отчёты","en":"Reports"}`)
}

func TestLinkPermissionCannotBroadenDeniedCard(t *testing.T) {
	card := testCard("protected")
	card.Links[0].PermissionKey = auth.NewPermissionKey("other", "read")
	registry, err := New(nil, []Card{card})
	require.NoError(t, err)
	request := registry.Authorize(testActor, func(key auth.PermissionKey) bool { return key.Domain == "other" })
	require.Empty(t, request.Descriptors().Cards)
}

func TestFailuresRemainUnavailableWithoutZeroMetrics(t *testing.T) {
	for _, allFail := range []bool{false, true} {
		t.Run(strconv.FormatBool(allFail), func(t *testing.T) {
			first := testCard("first")
			first.Load = func(context.Context, Actor) (Content, error) { return Content{}, errors.New("private database detail") }
			second := testCard("second")
			if allFail {
				second.Load = first.Load
			}
			registry, err := New(nil, []Card{first, second})
			require.NoError(t, err)
			var logged atomic.Int64
			request := registry.Authorize(testActor, func(auth.PermissionKey) bool { return true })
			loaded := request.Load(t.Context(), func(context.Context, string, error) { logged.Add(1) })
			require.Equal(t, "unavailable", loaded.Cards[0].State)
			require.Empty(t, loaded.Cards[0].Metrics)
			require.Empty(t, loaded.Cards[0].UpdatedAt)
			require.Nil(t, loaded.Cards[0].Status)
			require.Equal(t, "first", loaded.Cards[0].Key)
			expected := "ready"
			failures := int64(1)
			if allFail {
				expected = "unavailable"
				failures = 2
			}
			require.Equal(t, expected, loaded.Cards[1].State)
			require.Equal(t, failures, logged.Load())
			data, err := json.Marshal(loaded)
			require.NoError(t, err)
			require.NotContains(t, string(data), "private database detail")
		})
	}
}

func TestProviderConcurrencyBoundAndOrder(t *testing.T) {
	cards := make([]Card, 9)
	started := make(chan struct{}, len(cards))
	release := make(chan struct{})
	var active, peak atomic.Int64
	for i := range cards {
		cards[i] = testCard(fmt.Sprintf("card%d", i))
		cards[i].Load = func(ctx context.Context, _ Actor) (Content, error) {
			now := active.Add(1)
			defer active.Add(-1)
			for old := peak.Load(); now > old; old = peak.Load() {
				if peak.CompareAndSwap(old, now) {
					break
				}
			}
			started <- struct{}{}
			select {
			case <-ctx.Done():
				return Content{}, ctx.Err()
			case <-release:
				return Content{}, nil
			}
		}
	}
	registry, err := New(nil, cards)
	require.NoError(t, err)
	request := registry.Authorize(testActor, func(auth.PermissionKey) bool { return true })
	completed := make(chan Snapshot, 1)
	go func() { completed <- request.Load(t.Context(), nil) }()
	for range MaxParallel {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("provider did not start")
		}
	}
	require.EqualValues(t, MaxParallel, active.Load())
	close(release)
	var loaded Snapshot
	select {
	case loaded = <-completed:
	case <-time.After(time.Second):
		t.Fatal("provider pool did not join")
	}
	require.LessOrEqual(t, peak.Load(), int64(MaxParallel))
	for i, card := range loaded.Cards {
		require.Equal(t, cards[i].Key, card.Key)
		require.Equal(t, "ready", card.State)
	}
}

func TestProviderDeadlineCancellationAndLateSuccess(t *testing.T) {
	const canceledParent = "canceled parent"
	for _, mode := range []string{"timeout", canceledParent, "late success"} {
		t.Run(mode, func(t *testing.T) {
			card := testCard("deadline")
			var calls atomic.Int64
			card.Load = func(ctx context.Context, actor Actor) (Content, error) {
				calls.Add(1)
				require.Equal(t, testActor, actor)
				_, ok := ctx.Deadline()
				require.True(t, ok)
				<-ctx.Done()
				return Content{Metrics: []Metric{{Key: "stale", Label: Text{Text: "Stale"}, Value: "0"}}}, nil
			}
			registry, err := New(nil, []Card{card})
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if mode == canceledParent {
				cancel()
			}
			loaded := registry.Authorize(testActor, func(auth.PermissionKey) bool { return true }).load(ctx, 10*time.Millisecond, nil)
			require.Equal(t, "unavailable", loaded.Cards[0].State)
			require.Empty(t, loaded.Cards[0].Metrics)
			if mode == canceledParent {
				require.Zero(t, calls.Load())
			} else {
				require.EqualValues(t, 1, calls.Load())
			}
		})
	}
}

func TestDefaultProviderDeadlineAndActorIsolation(t *testing.T) {
	card := testCard("actor")
	card.Load = func(ctx context.Context, actor Actor) (Content, error) {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.InDelta(t, ProviderTimeout.Seconds(), time.Until(deadline).Seconds(), .2)
		return Content{Metrics: []Metric{{Key: "owner", Label: Text{Text: "Owner"}, Value: strconv.FormatInt(actor.AdminID, 10)}}}, nil
	}
	registry, err := New(nil, []Card{card})
	require.NoError(t, err)
	for _, id := range []int64{1, 2} {
		actor := testActor
		actor.AdminID = id
		snapshot := registry.Authorize(actor, func(auth.PermissionKey) bool { return true }).Load(t.Context(), nil)
		require.Equal(t, strconv.FormatInt(id, 10), snapshot.Owner)
		require.Equal(t, strconv.FormatInt(id, 10), snapshot.Cards[0].Metrics[0].Value)
	}
}

func TestProviderPanicIsUnavailableAndOtherCardsStillLoad(t *testing.T) {
	broken := testCard("broken")
	broken.Load = func(context.Context, Actor) (Content, error) { panic("private panic detail") }
	registry, err := New(nil, []Card{broken, testCard("healthy")})
	require.NoError(t, err)
	var logged atomic.Int64
	request := registry.Authorize(testActor, func(auth.PermissionKey) bool { return true })
	snapshot := request.Load(t.Context(), func(_ context.Context, _ string, err error) {
		require.EqualError(t, err, "dashboard provider panicked")
		logged.Add(1)
	})
	require.EqualValues(t, 1, logged.Load())
	require.Equal(t, "unavailable", snapshot.Cards[0].State)
	require.Empty(t, snapshot.Cards[0].Metrics)
	require.Equal(t, "ready", snapshot.Cards[1].State)
	data, err := json.Marshal(snapshot)
	require.NoError(t, err)
	require.NotContains(t, string(data), "private panic detail")
}

func TestCancelInFlightPoolJoinsWorkersAndSkipsQueuedCallbacks(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var active, calls atomic.Int64
	started := make(chan struct{}, MaxParallel)
	cards := make([]Card, 10)
	for i := range cards {
		cards[i] = testCard(fmt.Sprintf("cancel%d", i))
		cards[i].Load = func(ctx context.Context, _ Actor) (Content, error) {
			calls.Add(1)
			active.Add(1)
			defer active.Add(-1)
			started <- struct{}{}
			<-ctx.Done()
			return Content{}, ctx.Err()
		}
	}
	registry, err := New(nil, cards)
	require.NoError(t, err)
	request := registry.Authorize(testActor, func(auth.PermissionKey) bool { return true })
	done := make(chan Snapshot, 1)
	go func() { done <- request.Load(ctx, nil) }()
	for range MaxParallel {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("workers did not start")
		}
	}
	cancel()
	select {
	case snapshot := <-done:
		for _, card := range snapshot.Cards {
			require.Equal(t, "unavailable", card.State)
			require.Empty(t, card.Metrics)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled workers did not join")
	}
	require.Zero(t, active.Load())
	require.EqualValues(t, MaxParallel, calls.Load())
}
