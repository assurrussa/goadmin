package server_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	logger "github.com/assurrussa/gologger"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp/fasthttputil"

	"github.com/assurrussa/goadmin/infrastructure/fiber/server"
)

const testHTTPOrigin = "http://admin"

func TestShutdownConcurrentClose(t *testing.T) {
	signal := server.NewShutdown()
	var callers sync.WaitGroup
	for range 32 {
		callers.Go(signal.Close)
	}
	callers.Wait()
	signal.Close()
	select {
	case <-signal.Read():
	default:
		t.Fatal("shutdown signal is still open")
	}
}

func TestServerHTTPTimeouts(t *testing.T) {
	defaults := newServer(t).App.Config()
	require.Equal(t, 10*time.Second, defaults.ReadTimeout)
	require.Equal(t, 10*time.Second, defaults.WriteTimeout)
	require.Equal(t, 120*time.Second, defaults.IdleTimeout)
	custom := newServer(t, server.WithReadTimeout(time.Second), server.WithWriteTimeout(2*time.Second),
		server.WithIdleTimeout(30*time.Second)).App.Config()
	require.Equal(t, time.Second, custom.ReadTimeout)
	require.Equal(t, 2*time.Second, custom.WriteTimeout)
	require.Equal(t, 30*time.Second, custom.IdleTimeout)
	for _, option := range []server.OptOptionsSetter{
		server.WithReadTimeout(-time.Second), server.WithWriteTimeout(-time.Second), server.WithIdleTimeout(-time.Second),
	} {
		_, err := server.New(server.NewOptions(logger.Default(), "localhost:8080", []string{testHTTPOrigin}, option))
		require.ErrorContains(t, err, "timeouts must not be negative")
	}
}

func TestRunCancelledBeforeStartDrainsConnectionsWithFreshContext(t *testing.T) {
	want := errors.New("connection shutdown failed")
	called := false
	srv := newServer(t, server.WithShutdownConnections(func(ctx context.Context) error {
		called = true
		require.NoError(t, ctx.Err())
		_, bounded := ctx.Deadline()
		require.True(t, bounded)
		return want
	}))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, srv.Run(ctx), want)
	require.True(t, called)
}

func TestRunStopsOnCancellation(t *testing.T) {
	srv := newServer(t)
	listener := fasthttputil.NewInmemoryListener()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- srv.Run(ctx, listener) }()

	client := &http.Client{Transport: &http.Transport{
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			return listener.Dial()
		},
	}, Timeout: time.Second}
	defer client.CloseIdleConnections()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://admin/ping", nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	cancel()
	waitRun(t, result, nil)
	select {
	case <-srv.ReadShutdown():
	default:
		t.Fatal("shutdown signal is still open")
	}
}

func TestRunReturnsWhenListenerStopsNormally(t *testing.T) {
	srv := newServer(t)
	listener := &observedListener{Listener: fasthttputil.NewInmemoryListener(), ready: make(chan struct{})}
	result := make(chan error, 1)
	go func() { result <- srv.Run(t.Context(), listener) }()
	waitReady(t, listener.ready)
	require.NoError(t, srv.App.Shutdown())
	waitRun(t, result, nil)
}

func TestListenerCompletionDrainsActiveRequest(t *testing.T) {
	srv := newServer(t)
	entered := make(chan struct{})
	srv.App.Get("/wait", func(c fiber.Ctx) error {
		close(entered)
		<-srv.ReadShutdown()
		return c.SendStatus(http.StatusNoContent)
	})
	listener := fasthttputil.NewInmemoryListener()
	result := make(chan error, 1)
	go func() { result <- srv.Run(t.Context(), listener) }()
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(context.Context, string, string) (net.Conn, error) { return listener.Dial() },
	}, Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://admin/wait", nil)
	require.NoError(t, err)
	responseDone := make(chan error, 1)
	go func() {
		response, requestErr := client.Do(request)
		if requestErr == nil {
			requestErr = response.Body.Close()
		}
		responseDone <- requestErr
	}()
	waitReady(t, entered)
	require.NoError(t, listener.Close())
	waitRun(t, result, fasthttputil.ErrInmemoryListenerClosed)
	waitRun(t, responseDone, nil)
}

func TestRunReturnsListenerFailure(t *testing.T) {
	srv := newServer(t)
	failure := errors.New("listener failed")
	listener := &failedListener{failure: failure}
	result := make(chan error, 1)
	go func() { result <- srv.Run(t.Context(), listener) }()
	waitRun(t, result, failure)
}

func TestRunCancellationDuringStartup(t *testing.T) {
	for range 20 {
		srv := newServer(t)
		listener := &observedListener{Listener: fasthttputil.NewInmemoryListener(), ready: make(chan struct{})}
		ctx, cancel := context.WithCancel(t.Context())
		result := make(chan error, 1)
		srv.App.Hooks().OnListen(func(fiber.ListenData) error {
			cancel()
			return nil
		})
		go func() { result <- srv.Run(ctx, listener) }()
		waitRun(t, result, nil)
	}
}

func newServer(t *testing.T, options ...server.OptOptionsSetter) *server.Server {
	t.Helper()
	options = append([]server.OptOptionsSetter{
		server.WithDisableStartupMessage(true),
		server.WithRegistered(func(app *fiber.App) {
			app.Get("/ping", func(c fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })
		}),
	}, options...)
	srv, err := server.New(server.NewOptions(logger.Default(), "localhost:8080", []string{testHTTPOrigin}, options...))
	require.NoError(t, err)
	return srv
}

func waitReady(t *testing.T, ready <-chan struct{}) {
	t.Helper()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("listener did not start")
	}
}

func waitRun(t *testing.T, result <-chan error, want error) {
	t.Helper()
	select {
	case err := <-result:
		require.ErrorIs(t, err, want)
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
	}
}

type observedListener struct {
	net.Listener
	ready chan struct{}
	once  sync.Once
}

func (l *observedListener) Accept() (net.Conn, error) {
	l.once.Do(func() { close(l.ready) })
	return l.Listener.Accept()
}

type failedListener struct {
	failure error
}

func (l *failedListener) Accept() (net.Conn, error) { return nil, l.failure }
func (*failedListener) Close() error                { return nil }
func (*failedListener) Addr() net.Addr              { return &net.TCPAddr{} }

func TestHTTPTimeoutDefaultsAndOverrides(t *testing.T) {
	for _, tc := range []struct {
		name                          string
		read, write, idle             time.Duration
		wantRead, wantWrite, wantIdle time.Duration
	}{
		{name: "defaults", wantRead: 10 * time.Second, wantWrite: 10 * time.Second, wantIdle: 120 * time.Second},
		{
			name: "overrides", read: time.Second, write: 2 * time.Second, idle: 3 * time.Second,
			wantRead: time.Second, wantWrite: 2 * time.Second, wantIdle: 3 * time.Second,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, err := server.New(server.NewOptions(logger.Discard(), "localhost:8080", []string{testHTTPOrigin},
				server.WithReadTimeout(tc.read), server.WithWriteTimeout(tc.write), server.WithIdleTimeout(tc.idle),
				server.WithRegistered(func(app *fiber.App) {
					app.Get("/", func(c fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })
				}),
			))
			require.NoError(t, err)
			cfg := srv.App.Config()
			require.Equal(t, tc.wantRead, cfg.ReadTimeout)
			require.Equal(t, tc.wantWrite, cfg.WriteTimeout)
			require.Equal(t, tc.wantIdle, cfg.IdleTimeout)
			require.NoError(t, srv.Shutdown(t.Context()))
		})
	}
}

func TestHTTPTimeoutRejectsNegative(t *testing.T) {
	for _, option := range []server.OptOptionsSetter{
		server.WithReadTimeout(-time.Second), server.WithWriteTimeout(-time.Second), server.WithIdleTimeout(-time.Second),
	} {
		_, err := server.New(server.NewOptions(logger.Discard(), "localhost:8080", []string{testHTTPOrigin}, option))
		require.ErrorContains(t, err, "timeouts must not be negative")
	}
}

func TestRunCancellationWaitsForConnectionShutdown(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	srv, err := server.New(server.NewOptions(logger.Discard(), "localhost:8080", []string{testHTTPOrigin},
		server.WithRegistered(func(app *fiber.App) {
			app.Get("/", func(c fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })
		}),
		server.WithShutdownConnections(func(ctx context.Context) error {
			once.Do(func() { close(entered) })
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}),
	))
	require.NoError(t, err)
	require.True(t, srv.App.Server().KeepHijackedConns)
	listener := &observedListener{Listener: fasthttputil.NewInmemoryListener(), ready: make(chan struct{})}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- srv.Run(ctx, listener) }()
	waitReady(t, listener.ready)
	cancel()
	waitReady(t, entered)
	select {
	case <-result:
		t.Fatal("Run returned before connection shutdown")
	default:
	}
	close(release)
	waitRun(t, result, nil)
}

func TestDirectAndServerShutdownShareOneDrain(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	var calls atomic.Int32
	srv, err := server.New(server.NewOptions(logger.Discard(), "localhost:8080", []string{testHTTPOrigin},
		server.WithDisableStartupMessage(true),
		server.WithRegistered(func(app *fiber.App) {
			app.Get("/", func(c fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })
		}),
		server.WithShutdownConnections(func(ctx context.Context) error {
			if calls.Add(1) == 1 {
				close(entered)
			}
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}),
	))
	require.NoError(t, err)
	listener := &observedListener{Listener: fasthttputil.NewInmemoryListener(), ready: make(chan struct{})}
	runDone := make(chan error, 1)
	go func() { runDone <- srv.Run(t.Context(), listener) }()
	waitReady(t, listener.ready)
	directDone := make(chan error, 1)
	go func() { directDone <- srv.App.Shutdown() }()
	waitReady(t, entered)
	// A canceled waiter must return while the direct App drain is still active.
	waitCtx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, srv.Shutdown(waitCtx), context.Canceled)
	require.EqualValues(t, 1, calls.Load())
	deadlineCtx, cancelDeadline := context.WithTimeout(t.Context(), 10*time.Millisecond)
	require.ErrorIs(t, srv.Shutdown(deadlineCtx), context.DeadlineExceeded)
	cancelDeadline()
	require.EqualValues(t, 1, calls.Load())
	serverDone := make(chan error, 1)
	go func() { serverDone <- srv.Shutdown(t.Context()) }()
	select {
	case <-directDone:
		t.Fatal("direct App shutdown returned before drain")
	default:
	}
	select {
	case <-serverDone:
		t.Fatal("Server shutdown returned before drain")
	default:
	}
	unblock()
	waitRun(t, directDone, nil)
	waitRun(t, serverDone, nil)
	waitRun(t, runDone, nil)
	require.EqualValues(t, 1, calls.Load())
	require.NoError(t, srv.Shutdown(t.Context()))
	require.EqualValues(t, 1, calls.Load())
}

func TestShutdownBeforeRunIsIdempotent(t *testing.T) {
	srv := newServer(t)
	require.NoError(t, srv.Shutdown(t.Context()))
	require.NoError(t, srv.Shutdown(t.Context()))
}

func TestRunDrainDeadlineStillStopsHTTP(t *testing.T) {
	srv := newServer(t, server.WithShutdownConnections(func(ctx context.Context) error {
		<-ctx.Done()
		// Keep the callback outstanding after both wait/drain timers fire.
		time.Sleep(25 * time.Millisecond)
		return ctx.Err()
	}))
	listener := &observedListener{Listener: fasthttputil.NewInmemoryListener(), ready: make(chan struct{})}
	t.Cleanup(func() { _ = listener.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- srv.Run(ctx, listener) }()
	waitReady(t, listener.ready)
	cancel()
	select {
	case err := <-result:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(7 * time.Second):
		t.Fatal("Run kept the HTTP listener open after the connection drain deadline")
	}
}
