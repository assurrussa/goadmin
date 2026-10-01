package host //nolint:testpackage // verifies private module preflight and worker ownership

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/postgres"
	"github.com/assurrussa/gowebsocket/eventstream"
	inmem "github.com/assurrussa/gowebsocket/eventstream/inmem"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp/fasthttputil"

	server "github.com/assurrussa/goadmin/infrastructure/fiber/server"
)

const (
	testModuleReports   = "reports"
	testModuleJobs      = "jobs"
	testModuleOne       = "one"
	testModuleTwo       = "two"
	assemblyAdminOrigin = "http://admin.test"
)

func TestRealtimeModuleCreatesIndependentOwnedStreams(t *testing.T) {
	module := RealtimeModule(nil)
	first, second := &assembly{}, &assembly{}
	require.NoError(t, module.configure(first))
	require.NoError(t, module.configure(second))
	require.NotSame(t, first.input.Services.EventStream, second.input.Services.EventStream)
	require.NotNil(t, first.shutdownEventStream)
	require.NotNil(t, second.shutdownEventStream)
	require.NoError(t, first.shutdownEventStream(t.Context()))
	_, err := first.input.Services.EventStream.Subscribe(t.Context(), eventstream.NewUserID())
	require.ErrorIs(t, err, inmem.ErrClosed)
	_, err = second.input.Services.EventStream.Subscribe(t.Context(), eventstream.NewUserID())
	require.NoError(t, err)
	require.NoError(t, second.shutdownEventStream(t.Context()))
}

func TestRuntimeClosesOwnedRealtimeBeforeRunAndPreservesBorrowedStream(t *testing.T) {
	for _, borrowed := range []bool{false, true} {
		t.Run(map[bool]string{false: "owned", true: "borrowed"}[borrowed], func(t *testing.T) {
			var stream EventStream
			if borrowed {
				stream = NewEventStream()
				t.Cleanup(func() { require.NoError(t, stream.Close()) })
			}
			a := &assembly{}
			require.NoError(t, RealtimeModule(stream).configure(a))
			srv, err := server.New(server.NewOptions(DiscardLogger(), "localhost:8080", []string{assemblyAdminOrigin},
				server.WithShutdownConnections(a.shutdownEventStream),
				server.WithRegistered(func(app *fiber.App) { app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") }) }),
			))
			require.NoError(t, err)
			runtime := &Runtime{Server: srv, done: make(chan struct{})}
			require.NoError(t, runtime.Close())
			require.NoError(t, runtime.Close())
			_, err = a.input.Services.EventStream.Subscribe(t.Context(), eventstream.NewUserID())
			if borrowed {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, inmem.ErrClosed)
			}
		})
	}
}

func TestAssemblyRejectsInvalidModulesBeforeConstructingDependencies(t *testing.T) {
	for _, test := range []struct {
		name    string
		modules []ModuleDescriptor
		message string
	}{
		{"duplicate", []ModuleDescriptor{{Key: testModuleReports}, {Key: testModuleReports}}, "duplicate module"},
		{"missing dependency", []ModuleDescriptor{
			{Key: testModuleReports, Requires: []string{testModuleJobs}},
		}, "missing module dependency"},
		{"cycle", []ModuleDescriptor{
			{Key: testModuleOne, Requires: []string{testModuleTwo}}, {Key: testModuleTwo, Requires: []string{testModuleOne}},
		}, "dependency cycle"},
		{"route collision", []ModuleDescriptor{
			{Key: testModuleOne, Routes: []string{testReportRoute}}, {Key: testModuleTwo, Routes: []string{testReportRoute}},
		}, "claimed by"},
		{"core collision", []ModuleDescriptor{{Key: testModuleReports, Routes: []string{"GET /auth/login"}}}, "claimed by core"},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			modules := make([]Module, 0, len(test.modules))
			for _, descriptor := range test.modules {
				modules = append(modules, Module{descriptor: descriptor, configure: func(*assembly) error { called = true; return nil }})
			}
			// Nil PostgreSQL proves errors precede core client/dependency construction.
			runtime, err := New(t.Context(), Config{}, Dependencies{}, modules...)
			require.Nil(t, runtime)
			require.ErrorContains(t, err, test.message)
			require.False(t, called)
		})
	}
}

func TestAssemblyAuthMailBorrowedOwnershipPreflight(t *testing.T) {
	sender := goauth.NotificationSenderFunc(func(context.Context, goauth.NotificationDelivery) error {
		t.Fatal("mail sender must not run during module configuration")
		return nil
	})
	for _, test := range []struct {
		name    string
		auth    *AuthAdapter
		config  AuthMailConfig
		message string
	}{
		{
			name: "borrowed worker without auth", config: AuthMailConfig{BorrowedWorker: true, Sender: sender},
			message: "requires borrowed Auth",
		},
		{
			name: "borrowed auth delivery disabled", auth: &AuthAdapter{}, config: AuthMailConfig{BorrowedWorker: true},
			message: "does not support managed mail",
		},
		{
			name: "borrowed sender override", auth: &AuthAdapter{deliveryEnabled: true},
			config: AuthMailConfig{BorrowedWorker: true, Sender: sender}, message: "owns its sender",
		},
		{
			name: "borrowed worker override", auth: &AuthAdapter{deliveryEnabled: true},
			config:  AuthMailConfig{BorrowedWorker: true, Worker: postgres.NotificationWorkerConfig{Workers: 1}},
			message: "owns its sender",
		},
		{
			name: "explicit supervised borrowed delivery", auth: &AuthAdapter{deliveryEnabled: true},
			config: AuthMailConfig{BorrowedWorker: true},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := &assembly{borrowedAuth: test.auth}
			err := AuthMailModule(test.config).configure(a)
			if test.message != "" {
				require.ErrorContains(t, err, test.message)
				require.Nil(t, a.mail)
			} else {
				require.NoError(t, err)
				require.NotNil(t, a.mail)
				require.True(t, a.mail.BorrowedWorker)
			}
			require.Empty(t, a.workers)
		})
	}
}

func TestAssemblyOrdersDependenciesAndCopiesDescriptors(t *testing.T) {
	capabilities, ordered, err := validateModules([]Module{QueuesModule(), JobsModule(JobsConfig{})})
	require.NoError(t, err)
	require.Equal(t, []string{testModuleJobs, "queues"}, []string{ordered[0].Descriptor().Key, ordered[1].Descriptor().Key})
	require.Equal(t, map[string]bool{testModuleJobs: true, "queues": true}, capabilities)
	module := QueuesModule()
	cloned := module.Descriptor()
	cloned.Requires[0] = "mutated"
	cloned.RouteNamespaces[0] = "/mutated"
	require.Equal(t, testModuleJobs, module.Descriptor().Requires[0])
	require.Equal(t, "/queues", module.Descriptor().RouteNamespaces[0])
}

type borrowedJobsProbe struct {
	Outbox
	calls atomic.Int32
}

func (b *borrowedJobsProbe) Run(context.Context) error { b.calls.Add(1); return nil }
func (b *borrowedJobsProbe) BeginDrain()               { b.calls.Add(1) }
func (b *borrowedJobsProbe) Close() error              { b.calls.Add(1); return nil }

func TestAssemblyDoesNotOwnBorrowedJobs(t *testing.T) {
	borrowed := &borrowedJobsProbe{}
	a := &assembly{}
	require.NoError(t, JobsModule(JobsConfig{Borrowed: borrowed}).configure(a))
	require.Same(t, borrowed, a.input.Services.Outbox)
	require.Empty(t, a.workers)
	runtime := &Runtime{workers: a.workers, done: make(chan struct{})}
	require.NoError(t, runtime.Close())
	require.Zero(t, borrowed.calls.Load())
}

func TestAssemblyWorkerFailureCancelsAndJoinsOwnedWorkers(t *testing.T) {
	httpServer, err := server.New(server.NewOptions(DiscardLogger(), "127.0.0.1:12345", []string{assemblyAdminOrigin},
		server.WithDisableStartupMessage(true),
		server.WithRegistered(func(app *fiber.App) { app.Get("/", func(c fiber.Ctx) error { return c.SendString("ok") }) }),
	))
	require.NoError(t, err)
	listener := fasthttputil.NewInmemoryListener()
	t.Cleanup(func() { _ = listener.Close() })
	started, joined := make(chan struct{}), make(chan struct{})
	want := errors.New("owned worker failed")
	var drained atomic.Int32
	runtime := &Runtime{Server: httpServer, done: make(chan struct{}), workers: []runtimeWorker{
		{name: "waiting", run: func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			close(joined)
			return ctx.Err()
		}, drain: func() { drained.Add(1) }},
		{name: "failing", run: func(context.Context) error { <-started; return want }},
	}}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.ErrorIs(t, runtime.Run(ctx, listener), want)
	select {
	case <-joined:
	default:
		t.Fatal("Run returned before its cancelled sibling joined")
	}
	require.EqualValues(t, 1, drained.Load())
	require.NoError(t, runtime.Close())
	require.ErrorContains(t, runtime.Run(ctx), "already started or closed")
}
