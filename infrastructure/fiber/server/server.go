package server

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	logger "github.com/assurrussa/gologger"
	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	mRecover "github.com/gofiber/fiber/v3/middleware/recover"

	"github.com/assurrussa/goadmin/infrastructure/fiber/middlewares"
	"github.com/assurrussa/goadmin/internal/httpsecurity"
)

type Handler interface {
	RegisterGroupRoutes(router fiber.Router, middlewares ...fiber.Handler)
}

type FnRegistered func(app *fiber.App)

const shutdownTimeout = 3 * time.Second

//go:generate options-gen -out-filename=server_options.gen.go -from-struct=Options
type Options struct {
	logger                logger.Logger `option:"mandatory" validate:"required"`
	addr                  string        `option:"mandatory" validate:"required,hostname_port"`
	allowOrigins          []string      `option:"mandatory" validate:"required,min=1"`
	shutdown              *Shutdown
	shutdownConnections   func(context.Context) error
	readTimeout           time.Duration
	writeTimeout          time.Duration
	idleTimeout           time.Duration
	errHandler            fiber.ErrorHandler
	registered            FnRegistered
	handlers              []Handler
	serverHeader          string        `default:"tgmulti"`
	prefork               bool          `default:"false"`
	caseSensitive         bool          `default:"true"`
	strictRouting         bool          `default:"true"`
	disableStartupMessage bool          `default:"false"`
	maxRequest            int           `default:"0"`
	expiration            time.Duration `default:"1s"`
	bodyLimit             int           `default:"0"`
	allowHeaders          []string
	allowMethods          []string
	allowCredentials      bool `default:"true"`
	cors                  *cors.Config
	tlsCert               string
	tlsKey                string
	middlewares           []fiber.Handler
	skipFnLogger          *middlewares.RequestLoggerConfig
}

type Server struct {
	lg                    logger.Logger
	addr                  string
	tlsCert               string
	tlsKey                string
	disableStartupMessage bool
	enablePrefork         bool
	App                   *fiber.App
	// Register custom shutdown function to stop long-living connections like websockets.
	shutdown                *Shutdown
	shutdownConnections     func(context.Context) error
	connectionsShutdownOnce sync.Once
	connectionsShutdownDone chan struct{}
	connectionsShutdownErr  error
	httpShutdownOnce        sync.Once
	httpShutdownDone        chan struct{}
	httpShutdownErr         error
}

func New(opts Options) (*Server, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("validate HTTP server options: %w", err)
	}
	if err := httpsecurity.ValidateTLSFiles(opts.tlsCert, opts.tlsKey); err != nil {
		return nil, err
	}
	if opts.readTimeout < 0 || opts.writeTimeout < 0 || opts.idleTimeout < 0 {
		return nil, errors.New("HTTP timeouts must not be negative")
	}
	if opts.readTimeout == 0 {
		opts.readTimeout = 10 * time.Second
	}
	if opts.writeTimeout == 0 {
		opts.writeTimeout = 10 * time.Second
	}
	if opts.idleTimeout == 0 {
		opts.idleTimeout = 120 * time.Second
	}

	app := fiber.New(fiber.Config{
		ServerHeader: opts.serverHeader, CaseSensitive: opts.caseSensitive,
		StrictRouting: opts.strictRouting, ErrorHandler: opts.errHandler,
		BodyLimit: opts.bodyLimit, JSONEncoder: json.Marshal, JSONDecoder: json.Unmarshal,
		ReadTimeout: opts.readTimeout, WriteTimeout: opts.writeTimeout, IdleTimeout: opts.idleTimeout,
	})

	// A managed WebSocket handler closes the underlying hijacked connection to
	// interrupt its read pump before returning. fasthttp otherwise makes Close a
	// no-op until the hijack callback returns, which deadlocks that drain.
	if opts.shutdownConnections != nil {
		app.Server().KeepHijackedConns = true
	}

	logConfig := middlewares.RequestLoggerConfig{Logger: opts.logger}
	if opts.skipFnLogger != nil {
		logConfig = *opts.skipFnLogger
		if logConfig.Logger == nil {
			logConfig.Logger = opts.logger
		}
	}
	app.Use(middlewares.NewRequestLogger(logConfig))
	app.Use(mRecover.New())
	for _, middleware := range opts.middlewares {
		app.Use(middleware)
	}

	if opts.cors != nil {
		app.Use(cors.New(*opts.cors))
	} else if opts.allowOrigins != nil {
		allowMethods := []string{
			fiber.MethodGet, fiber.MethodPost, fiber.MethodHead, fiber.MethodPut, fiber.MethodDelete, fiber.MethodPatch,
		}
		if len(opts.allowMethods) > 0 {
			allowMethods = opts.allowMethods
		}
		app.Use(cors.New(cors.Config{
			AllowOrigins: opts.allowOrigins, AllowHeaders: opts.allowHeaders,
			AllowMethods: allowMethods, AllowCredentials: opts.allowCredentials,
		}))
	}
	if opts.maxRequest > 0 {
		app.Use(limiter.New(limiter.Config{
			Max: opts.maxRequest, Expiration: opts.expiration,
			KeyGenerator: func(c fiber.Ctx) string { return c.IP() },
		}))
	}
	if opts.registered == nil && len(opts.handlers) == 0 {
		return nil, errors.New("no registered handlers")
	}
	if opts.registered != nil {
		opts.registered(app)
	}
	for _, handler := range opts.handlers {
		handler.RegisterGroupRoutes(app)
	}

	shutdown := NewShutdown()
	if opts.shutdown != nil {
		shutdown = opts.shutdown
	}
	srv := &Server{
		lg: opts.logger, addr: opts.addr, tlsCert: opts.tlsCert, tlsKey: opts.tlsKey,
		disableStartupMessage: opts.disableStartupMessage, enablePrefork: opts.prefork,
		App: app, shutdown: shutdown, shutdownConnections: opts.shutdownConnections,
		connectionsShutdownDone: make(chan struct{}),
		httpShutdownDone:        make(chan struct{}),
	}
	app.Hooks().OnPreShutdown(func() error {
		shutdown.Close()
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return srv.drainConnections(ctx)
	})
	return srv, nil
}

func (s *Server) ReadShutdown() <-chan struct{} { return s.shutdown.Read() }

// Shutdown drains long-lived connections before stopping the HTTP listener.
// Direct App shutdown and Server shutdown share one drain. Each caller's context
// bounds its wait without canceling the shared, bounded connection cleanup.
func (s *Server) Shutdown(ctx context.Context) error {
	s.shutdown.Close()
	// Cleanup continues if an individual caller stops waiting. In particular,
	// an expired drain budget must never leave Run blocked in App.Listener.
	s.httpShutdownOnce.Do(func() {
		go func() {
			drainCtx, cancelDrain := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
			connectionErr := s.drainConnections(drainCtx)
			cancelDrain()
			httpCtx, cancelHTTP := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
			err := s.App.ShutdownWithContext(httpCtx)
			cancelHTTP()
			if errors.Is(err, fiber.ErrNotRunning) {
				err = nil
			}
			s.httpShutdownErr = errors.Join(connectionErr, err)
			close(s.httpShutdownDone)
		}()
	})
	select {
	case <-s.httpShutdownDone:
		return s.httpShutdownErr
	default:
	}
	select {
	case <-s.httpShutdownDone:
		return s.httpShutdownErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) drainConnections(ctx context.Context) error {
	s.connectionsShutdownOnce.Do(func() {
		go func() {
			drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
			defer cancel()
			if s.shutdownConnections != nil {
				s.connectionsShutdownErr = s.shutdownConnections(drainCtx)
			}
			close(s.connectionsShutdownDone)
		}()
	})
	select {
	case <-s.connectionsShutdownDone:
		return s.connectionsShutdownErr
	default:
	}
	select {
	case <-s.connectionsShutdownDone:
		return s.connectionsShutdownErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) shutdownWithTimeout(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	return s.Shutdown(ctx)
}
