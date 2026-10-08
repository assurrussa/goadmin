package host

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"sync"
	"time"

	"github.com/assurrussa/goauth"
	"github.com/gofiber/fiber/v3"
	"golang.org/x/sync/errgroup"

	"github.com/assurrussa/goadmin/bootstrap"
)

type ServerConfig = bootstrap.ServerConfig

// Config describes the core. Optional infrastructure is configured by modules.
type Config struct {
	Admin                AdminConfig
	Server               ServerConfig
	Auth                 goauth.Config
	CSRF                 CSRFConfig
	FirstAdminSetupToken FirstAdminSetupToken
	Templates            fs.FS
	Public               fs.ReadFileFS
	Options              []Option
}

// Dependencies are borrowed. New creates admin-owned adapter/session wrappers
// when Auth/Sessions are omitted; it never closes the host PostgreSQL/Redis pool.
type Dependencies struct {
	ExternalAuthority  ExternalSessionAuthority
	Database           StoragePgsqlClient
	TxManager          StoragePgsqlTxManager
	Logger             Logger
	Auth               *AuthAdapter
	Sessions           *SessionStore
	SessionRedis       RedisClient
	CSRF               *CSRFService
	LoginAudit         LoginAuditWriter
	SubjectPermissions *SubjectPermissionChecker
}

type assembly struct {
	input               assemblyInput
	borrowedAuth        *AuthAdapter
	mail                *AuthMailConfig
	workers             []runtimeWorker
	shutdownEventStream func(context.Context) error
}
type runtimeWorker struct {
	name  string
	run   func(context.Context) error
	ready func(context.Context) error
	drain func()
}

// Runtime owns assembled workers and HTTP lifetime. Construction is inert.
type Runtime struct {
	Server    *Server
	Auth      *AuthAdapter
	workers   []runtimeWorker
	ownedAuth bool
	mu        sync.Mutex
	running   bool
	started   bool
	closed    bool
	cancel    context.CancelFunc
	done      chan struct{}
	closeOnce sync.Once
	closeErr  error
}

// New assembles an admin core with exactly the requested modules.
func New(ctx context.Context, cfg Config, deps Dependencies, modules ...Module) (_ *Runtime, err error) {
	capabilities, ordered, err := validateModules(modules)
	if err != nil {
		return nil, err
	}
	if deps.Database == nil || deps.Database.DB() == nil || deps.Database.DB().Pool() == nil {
		return nil, errors.New("admin core requires PostgreSQL")
	}
	cfg, deps = normalizeCoreConfig(cfg, deps)
	a := assembly{borrowedAuth: deps.Auth, input: assemblyInput{
		Config: BootstrapConfig{Env: cfg.Admin.Env, AdminConfig: cfg.Admin, ServerConfig: cfg.Server, Capabilities: capabilities},
		Services: Services{
			DB: deps.Database, TxManager: deps.TxManager, Logger: deps.Logger, CSRF: deps.CSRF,
			SessionStore: deps.Sessions, SessionRedis: deps.SessionRedis, SubjectPermissions: deps.SubjectPermissions,
			ExternalAuthority: deps.ExternalAuthority,
		},
		Repositories: Repositories{AdminLoginAudit: deps.LoginAudit}, FirstAdminSetupToken: cfg.FirstAdminSetupToken,
		Options: append([]Option{}, cfg.Options...),
	}}
	defer func() {
		a.closeEventStreamOnError(ctx, err)
	}()
	features := make([]Feature, 0, len(ordered))
	for _, m := range ordered {
		if m.configure != nil {
			if err = m.configure(&a); err != nil {
				return nil, fmt.Errorf("module %s: %w", m.descriptor.Key, err)
			}
		}
		if m.feature != nil {
			features = append(features, m.feature)
		}
	}
	registry := NewRegistry(features...)
	bundle, err := BuildClientBundleValidation(registry, cfg.Public)
	if err != nil {
		return nil, err
	}
	a.input.ClientBundle = bundle
	a.input.Options = append(a.input.Options, BuildBootstrapOptions(BootstrapOptionsInput{
		Env: cfg.Admin.Env, Registry: registry, PublicFS: cfg.Public, TemplateFS: cfg.Templates,
	})...)
	r := &Runtime{Auth: deps.Auth, done: make(chan struct{})}
	defer func() {
		if err != nil && r.ownedAuth {
			_ = r.Auth.Close()
		}
	}()
	if r.Auth == nil {
		authCfg := AuthAdapterConfig{Database: deps.Database, TxManager: deps.TxManager, Runtime: cfg.Auth}
		authCfg.Runtime.NotificationDelivery = goauth.NotificationDeliveryDisabled
		if a.mail != nil {
			authCfg.Runtime.NotificationDelivery = goauth.NotificationDeliveryRequired
			authCfg.NotificationSender = a.mail.Sender
			authCfg.NotificationWorker = a.mail.Worker
		}
		r.Auth, err = NewAuthAdapter(authCfg)
		if err != nil {
			return nil, err
		}
		r.ownedAuth = true
	}
	if err = validateAssemblyAuth(r.Auth, deps.Database); err != nil {
		return nil, err
	}
	a.input.Auth = r.Auth
	if err = a.initializeSessions(cfg.Admin, r.Auth); err != nil {
		return nil, err
	}
	if a.input.Services.CSRF == nil {
		a.input.Services.CSRF, err = NewCSRFService(cfg.CSRF, deps.Logger)
		if err != nil {
			return nil, err
		}
	}
	if a.mail != nil && !a.mail.BorrowedWorker {
		a.workers = append(a.workers, runtimeWorker{name: authMailModuleKey, run: r.Auth.Runtime().RunNotifications})
	}
	built, err := buildDependencies(a.input)
	if err != nil {
		return nil, err
	}
	built.System.ShutdownEventStream = a.shutdownEventStream
	r.Server, err = bootstrap.Run(ctx, a.input.Config, built, a.input.Options...)
	if err != nil {
		return nil, err
	}
	r.workers = a.workers
	return r, nil
}

func (a *assembly) closeEventStreamOnError(ctx context.Context, err error) {
	if err == nil || a.shutdownEventStream == nil {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	_ = a.shutdownEventStream(cleanupCtx)
}

// Run starts each owned worker once. A worker exiting unexpectedly cancels and
// joins every sibling and the server before Run returns.
func (r *Runtime) Run(ctx context.Context, listeners ...net.Listener) error {
	r.mu.Lock()
	if r.closed || r.started {
		r.mu.Unlock()
		return errors.New("admin runtime already started or closed")
	}
	r.started = true
	r.running = true
	runCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.mu.Unlock()
	defer func() { cancel(); r.mu.Lock(); r.running = false; close(r.done); r.mu.Unlock() }()
	group, workerCtx := errgroup.WithContext(runCtx)
	group.Go(func() error {
		err := r.Server.Run(workerCtx, listeners...)
		if err == nil && workerCtx.Err() == nil {
			return errors.New("admin server exited unexpectedly")
		}
		return err
	})
	for _, worker := range r.workers {
		group.Go(func() error {
			err := worker.run(workerCtx)
			if workerCtx.Err() != nil && (err == nil || errors.Is(err, context.Canceled)) {
				return nil
			}
			if err == nil {
				err = errors.New("worker exited unexpectedly")
			}
			return fmt.Errorf("admin %s: %w", worker.name, err)
		})
	}
	group.Go(func() error {
		<-workerCtx.Done()
		for _, w := range r.workers {
			if w.drain != nil {
				w.drain()
			}
		}
		return nil
	})
	err := group.Wait()
	if ctx.Err() != nil && (err == nil || errors.Is(err, context.Canceled)) {
		return nil
	}
	return err
}

func (r *Runtime) Readiness(ctx context.Context) error {
	r.mu.Lock()
	running := r.running && !r.closed
	r.mu.Unlock()
	if !running {
		return errors.New("admin runtime is not running")
	}
	if err := r.Auth.Runtime().Database().PingContext(ctx); err != nil {
		return err
	}
	for _, w := range r.workers {
		if w.ready != nil {
			if err := w.ready(ctx); err != nil {
				return fmt.Errorf("admin %s readiness: %w", w.name, err)
			}
		}
	}
	return nil
}

// Close cancels and joins a running runtime, then closes only owned wrappers.
func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.closed = true
		started := r.started
		if r.cancel != nil {
			r.cancel()
		}
		r.mu.Unlock()
		if started {
			<-r.done
		}
		if r.Server != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			r.closeErr = r.Server.Shutdown(ctx)
			cancel()
		}
		if r.ownedAuth {
			r.closeErr = errors.Join(r.closeErr, r.Auth.Close())
		}
	})
	return r.closeErr
}

// App exposes the underlying Fiber app for host tests; product features use the SDK.
func (r *Runtime) App() *fiber.App { return r.Server.App }

func normalizeCoreConfig(cfg Config, deps Dependencies) (Config, Dependencies) {
	if deps.Logger == nil {
		deps.Logger = DiscardLogger()
	}
	if deps.TxManager == nil {
		deps.TxManager = NewTxManager(deps.Database)
	}
	if cfg.Admin.CanonicalSessionBackend == "" {
		cfg.Admin.CanonicalSessionBackend = "pgsql"
	}
	if cfg.Admin.CSRFTokenName == "" {
		cfg.Admin.CSRFTokenName = "csrf_token"
	}
	if cfg.Server.Addr == "" {
		cfg.Server.Addr = cfg.Admin.Addr
	}
	if len(cfg.Server.AllowOrigins) == 0 {
		cfg.Server.AllowOrigins = []string{cfg.Admin.DomainURL}
	}
	if cfg.Public == nil {
		cfg.Public = CorePublicFS()
	}
	if cfg.Templates == nil {
		cfg.Templates = CoreTemplateFS()
	}
	return cfg, deps
}

func (a *assembly) initializeSessions(cfg AdminConfig, auth *AuthAdapter) error {
	if a.input.Services.SessionStore != nil {
		return nil
	}
	if cfg.CanonicalSessionBackend == "redis" {
		if a.input.Services.SessionRedis == nil {
			return errors.New("redis session backend requires SessionRedis")
		}
		_, a.input.Services.SessionStore = CreateSessionStore(a.input.Services.SessionRedis, cfg)
		return nil
	}
	sessions, err := NewPostgresSessions(cfg, auth.Runtime().Database())
	if err != nil {
		return err
	}
	a.input.Services.SessionStore = sessions.Store
	a.workers = append(a.workers, runtimeWorker{name: "sessions", run: sessions.Run})
	return nil
}

func validateAssemblyAuth(auth *AuthAdapter, database StoragePgsqlClient) error {
	if auth.database == nil || auth.database.DB().Pool() != database.DB().Pool() {
		return errors.New("borrowed auth and admin core require the same PostgreSQL pool")
	}
	if !auth.valid() {
		return errors.New("admin auth adapter is invalid")
	}
	return nil
}
