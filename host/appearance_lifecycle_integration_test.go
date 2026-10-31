//go:build integration

package host //nolint:testpackage // Verifies the assembled core's private dependency ownership.

import (
	"context"
	"database/sql"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/internal/identity"
)

// Keep real PostgreSQL execution and block only an admitted appearance read.
// The runtime itself must create, own and stop its appearance service.
type appearanceLifecycleDB struct {
	outbox.StoragePgsqlDBEngine
	armed             atomic.Bool
	polls             atomic.Int64
	entered           chan struct{}
	release           chan struct{}
	once              sync.Once
	auth              *sql.DB
	dependencyErr     chan error
	pollArmed         atomic.Bool
	pollOnce          sync.Once
	pollEntered       chan struct{}
	pollCanceled      chan struct{}
	pollRelease       chan struct{}
	pollDependencyErr chan error
}

func (d *appearanceLifecycleDB) ScanOne(ctx context.Context, op string, dest any, query string, args ...any) error {
	if op == "branding.version" {
		d.polls.Add(1)
		if d.pollArmed.Load() {
			d.pollOnce.Do(func() {
				close(d.pollEntered)
				<-ctx.Done()
				close(d.pollCanceled)
				<-d.pollRelease
				d.pollDependencyErr <- d.auth.PingContext(context.Background())
			})
		}
	}
	if op == "admin_preferences.load" && len(args) == 2 && args[1] == "ui.appearance" && d.armed.Load() {
		d.once.Do(func() {
			close(d.entered)
			<-d.release
			// This query executes inside the admitted operation before it drains.
			// Closing the owned auth DB first would make it fail with sql.ErrDBClosed.
			d.dependencyErr <- d.auth.PingContext(context.Background())
		})
	}
	return d.StoragePgsqlDBEngine.ScanOne(ctx, op, dest, query, args...)
}

type appearanceLifecycleClient struct {
	StoragePgsqlClient
	engine outbox.StoragePgsqlDBEngine
}

func (d appearanceLifecycleClient) DB() outbox.StoragePgsqlDBEngine { return d.engine }

func TestAssembledAppearanceDrainsBeforeAuthCloseAndStopsPolling(t *testing.T) {
	for _, run := range []bool{false, true} {
		t.Run(map[bool]string{false: "close_before_Run", true: "running"}[run], func(t *testing.T) {
			cfg, deps := assemblyFixture(t)
			probe := &appearanceLifecycleDB{StoragePgsqlDBEngine: deps.Database.DB(), entered: make(chan struct{}), release: make(chan struct{}), dependencyErr: make(chan error, 1), pollEntered: make(chan struct{}), pollCanceled: make(chan struct{}), pollRelease: make(chan struct{}), pollDependencyErr: make(chan error, 1)}
			deps.Database = appearanceLifecycleClient{StoragePgsqlClient: deps.Database, engine: probe}
			runtime, err := New(t.Context(), cfg, deps)
			require.NoError(t, err)
			// Release first on assertion failure, so cleanup never strands owned work.
			var release, releasePoll sync.Once
			t.Cleanup(func() {
				release.Do(func() { close(probe.release) })
				releasePoll.Do(func() { close(probe.pollRelease) })
				require.NoError(t, runtime.Close())
			})
			probe.auth = runtime.Auth.Runtime().Database()
			account, err := runtime.Auth.Runtime().ProvisionTrustedLocalAccount(t.Context(), goauth.RegisterRequest{Email: "appearance-lifecycle@example.test", Password: assemblyPassword})
			require.NoError(t, err)
			_, err = runtime.Auth.admins.ProvisionAccount(t.Context(), account, identity.NewUserID())
			require.NoError(t, err)
			cookies := make(map[string]*http.Cookie)
			response, body := assemblyGet(t, runtime, "/auth/login", cookies)
			require.Equal(t, http.StatusOK, response.StatusCode, string(body))
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, assemblyAdminOrigin+"/auth/login", strings.NewReader(`{"email":"appearance-lifecycle@example.test","password":"`+assemblyPassword+`"}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Inertia", "true")
			request.Header.Set("Origin", assemblyAdminOrigin)
			request.Header.Set("X-CSRF-Token", cookies["csrf_token"].Value)
			response, body = assemblyRequest(t, runtime, request, cookies)
			require.Contains(t, []int{http.StatusFound, http.StatusSeeOther}, response.StatusCode, string(body))
			// Observe a real periodic version check before shutdown, rather than just
			// testing a generic finalizer or an injected appearance service.
			require.Eventually(t, func() bool { return probe.polls.Load() >= 2 }, 4*time.Second, 10*time.Millisecond)
			runDone := make(chan error, 1)
			if run {
				listener, e := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, e)
				go func() { runDone <- runtime.Run(context.Background(), listener) }()
				require.Eventually(t, func() bool { runtime.mu.Lock(); defer runtime.mu.Unlock(); return runtime.started }, time.Second, time.Millisecond)
			}
			probe.armed.Store(true)
			type responseResult struct {
				response *http.Response
				err      error
			}
			requestDone := make(chan responseResult, 1)
			profileRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, assemblyAdminOrigin+"/auth/profile", nil)
			profileRequest.Header.Set("X-Inertia", "true")
			for _, cookie := range cookies {
				profileRequest.AddCookie(cookie)
			}
			go func() {
				response, e := runtime.App().Test(profileRequest, fiber.TestConfig{Timeout: 0, FailOnTimeout: false})
				requestDone <- responseResult{response, e}
			}()
			select {
			case <-probe.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("assembled appearance read was not admitted")
			}
			probe.pollArmed.Store(true)
			select {
			case <-probe.pollEntered:
			case <-time.After(4 * time.Second):
				t.Fatal("periodic appearance refresh did not enter")
			}
			closed := make(chan error, 1)
			go func() { closed <- runtime.Close() }()
			select {
			case e := <-closed:
				t.Fatalf("runtime closed before admitted appearance read drained: %v", e)
			case <-time.After(100 * time.Millisecond):
			}
			require.NoError(t, probe.auth.PingContext(t.Context()), "owned auth must remain available while appearance drains")
			release.Do(func() { close(probe.release) })
			require.NoError(t, <-probe.dependencyErr)
			select {
			case <-probe.pollCanceled:
			case <-time.After(2 * time.Second):
				t.Fatal("appearance polling was not canceled after draining")
			}
			select {
			case e := <-closed:
				t.Fatalf("runtime did not join in-flight RCU loader: %v", e)
			case <-time.After(100 * time.Millisecond):
			}
			require.NoError(t, probe.auth.PingContext(t.Context()), "auth must remain available while the RCU loader exits")
			releasePoll.Do(func() { close(probe.pollRelease) })
			require.NoError(t, <-probe.pollDependencyErr)
			select {
			case result := <-requestDone:
				require.NoError(t, result.err)
				require.Equal(t, http.StatusOK, result.response.StatusCode)
				require.NoError(t, result.response.Body.Close())
			case <-time.After(5 * time.Second):
				t.Fatal("admitted appearance read did not finish")
			}
			select {
			case e := <-closed:
				require.NoError(t, e)
			case <-time.After(5 * time.Second):
				t.Fatal("runtime did not join appearance cleanup")
			}
			if run {
				select {
				case <-runDone:
				case <-time.After(time.Second):
					t.Fatal("Run did not join")
				}
			}
			require.Error(t, probe.auth.PingContext(t.Context()), "owned auth DB should now be closed")
			after := probe.polls.Load()
			time.Sleep(2200 * time.Millisecond)
			require.Equal(t, after, probe.polls.Load(), "appearance RCU continued polling after runtime Close")
			require.NoError(t, deps.Database.DB().Ping(t.Context()), "borrowed host PostgreSQL remains available")
		})
	}
}
