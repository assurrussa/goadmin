package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	uploadhost "github.com/assurrussa/gouploads/host"
	handlers "github.com/assurrussa/gowebsocket/websocketstream/handlers"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/favicon"
	"github.com/gofiber/fiber/v3/middleware/static"

	"github.com/assurrussa/goadmin/adminapp"
	adminadmins "github.com/assurrussa/goadmin/http/handlers/administrations"
	adminauthhandler "github.com/assurrussa/goadmin/http/handlers/auth"
	adminmain "github.com/assurrussa/goadmin/http/handlers/dashboard"
	adminfiles "github.com/assurrussa/goadmin/http/handlers/files"
	adminnotificationshandler "github.com/assurrussa/goadmin/http/handlers/notifications"
	adminpermissions "github.com/assurrussa/goadmin/http/handlers/permissions"
	adminprofileavatar "github.com/assurrussa/goadmin/http/handlers/profileavatar"
	adminqueues "github.com/assurrussa/goadmin/http/handlers/queues"
	adminroleshandler "github.com/assurrussa/goadmin/http/handlers/roles"
	adminmiddleware "github.com/assurrussa/goadmin/infrastructure/core/middlewares"
	middlewares "github.com/assurrussa/goadmin/infrastructure/fiber/middlewares"
	server "github.com/assurrussa/goadmin/infrastructure/fiber/server"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/uploadintegration"
	adminshared "github.com/assurrussa/goadmin/shared"
)

// initServer encapsulates the HTTP Server initialization.
func initServer(ctx context.Context, deps Dependencies, cfg Config, app *adminapp.App, opts bootOptions) (*server.Server, error) {
	_ = ctx
	if deps.System.FirstAdminSetup == nil {
		return nil, errors.New("first admin setup token service is required")
	}

	// Upload Context Builder
	uploadContextBuild := func(ctx context.Context, metadata map[string]string) (uploadhost.UploadContext, error) {
		adminAuth := adminshared.GetAdminAuth(ctx)
		if adminAuth == nil {
			return uploadhost.UploadContext{}, nil
		}

		return uploadhost.UploadContext{
			UserID:    adminAuth.ID,
			UserUUID:  uploadhost.UserID(adminAuth.UUID),
			SessionID: adminAuth.SessionID,
			Metadata:  metadata,
		}, nil
	}

	shutdownFn := server.NewShutdown()

	var httpWS *handlers.HTTPHandler
	var err error
	if cfg.Enabled("realtime") {
		httpWS, err = newRealtimeHandler(deps.System.Logger, deps.System.EventStream, cfg.ServerConfig.AllowOrigins)
		if err != nil {
			return nil, fmt.Errorf("create websocket handler: %w", err)
		}
	}
	shutdownRealtime := realtimeShutdown(httpWS, deps.System.ShutdownEventStream)
	assembled := false
	defer func() {
		if !assembled {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer cancel()
			_ = shutdownRealtime(ctx)
		}
	}()

	// Upload Handler
	// Note: deps.Repos.FileRepo acts as UploadFileRepo if not specified otherwise
	var adminFileRepo adminfiles.FileRepository
	var uploadsHandler *uploadhost.FiberUploadHandler
	if cfg.Enabled("uploads") {
		adminFileRepo = adminfiles.NewManagerScopedFileRepository(deps.Repos.FileRepo)
		uploadsHandler = uploadhost.NewFiberUploadHandler(uploadhost.NewUploadHandler(
			deps.Uploads.Service,
			adminFileRepo,
			deps.Uploads.TusStore,
			deps.System.Logger,
			uploadContextBuild,
			app.ComposeFileURL,
		))

		if err := registerUploadStrategies(uploadsHandler, deps.System.AdminRepo, deps.Uploads.Strategies); err != nil {
			return nil, err
		}
		if err := validateUploadTransports(opts.uploadTransports); err != nil {
			return nil, err
		}
	}

	extensions, err := buildServerExtensions(app, opts.extensionBuilders)
	if err != nil {
		return nil, err
	}

	// Define Middlewares
	defaultMiddlewares := make([]fiber.Handler, 0, 3+len(extensions.middlewares))
	defaultMiddlewares = append(defaultMiddlewares,
		middlewares.NewRequestID(),
		responseCompression(opts.publicFS != nil),
		favicon.New(),
	)
	defaultMiddlewares = append(defaultMiddlewares, extensions.middlewares...)

	// Core routes are always mounted; module routes use the same capability snapshot.
	httpHandlers := []server.Handler{adminmain.NewHandler(app), adminauthhandler.NewHandler(adminauthhandler.HandlerOptions{
		AdminApp:            app,
		Sleep:               opts.authSleep,
		AdminRepo:           deps.System.AdminRepo,
		RolesManager:        deps.Repos.RolesManager,
		PreviewService:      deps.Repos.FileLoader,
		AdminAccountService: deps.System.AdminAuthService,
		SessionStoreActions: deps.System.AdminAuthService,
		AdminLoginAudit:     deps.Repos.AdminLoginAudit,
		FirstAdminSetup:     deps.System.FirstAdminSetup,
		ActionAudit:         deps.System.AdminActionAudit,
		AdminDomainURL:      cfg.AdminConfig.DomainURL,
		AdminMenu:           opts.menu,
	})}
	if cfg.Enabled("uploads") {
		httpHandlers = append(httpHandlers,
			adminprofileavatar.NewHandler(deps.Uploads.Service, deps.System.Logger),
			adminfiles.NewHandler(app, uploadsHandler, adminFileRepo),
		)
	}
	if cfg.Enabled("access") {
		useCases, buildErr := adminroleshandler.BuildUseCases(deps.Repos.RolesManager, deps.Repos.RolesRepo, deps.System.AdminRepo)
		if buildErr != nil {
			return nil, fmt.Errorf("build access use cases: %w", buildErr)
		}
		if buildErr = validateRolesUseCases(useCases); buildErr != nil {
			return nil, buildErr
		}
		httpHandlers = append(httpHandlers,
			adminadmins.NewHandler(app, deps.System.AdminRepo, deps.System.AdminAuthService,
				deps.Repos.FileRepo, deps.Repos.FileLoader, deps.System.AdminActionAudit),
			adminroleshandler.NewHandler(app, deps.System.AdminRepo, useCases),
			adminpermissions.NewHandler(app, useCases.ListPermissions, deps.Repos.RolesManager),
		)
	}
	if cfg.Enabled("queues") {
		httpHandlers = append(httpHandlers, adminqueues.NewHandler(
			app, deps.Repos.JobsRepo, deps.Repos.JobsFailedRepo, deps.System.AdminActionAudit,
		))
	}
	if cfg.Enabled("notifications") {
		httpHandlers = append(httpHandlers, adminnotificationshandler.NewHandler(app, deps.System.AdminNotificationRepo))
	}

	httpHandlers = append(httpHandlers, extensions.handlers...)

	uploadsRoot := cfg.AdminConfig.StaticDataPath("uploads")
	staticRoutes := make([]StaticRoute, 0, len(opts.staticRoutes)+len(extensions.staticRoutes))
	staticRoutes = append(staticRoutes, opts.staticRoutes...)

	staticRoutes = append(staticRoutes, extensions.staticRoutes...)

	//nolint:contextcheck // Fiber registered hook does not expose an outer context to thread through middlewares.
	fnRegistered := buildRegisteredHandler(
		deps,
		app,
		opts,
		httpWS,
		uploadContextBuild,
		staticFilesConfig{
			routes: staticRoutes, uploadsRoot: filepath.Clean(uploadsRoot),
			csrfCookieName: getCookieTokenName(cfg.AdminConfig),
		},
		extensions.publicRegistered,
		extensions.registered,
	)

	// Build Server
	//nolint:contextcheck // Server construction has no context; its shutdown hooks own bounded cleanup contexts.
	srv, err := server.New(server.NewOptions(
		deps.System.Logger,
		cfg.ServerConfig.Addr,
		cfg.ServerConfig.AllowOrigins,
		server.WithRegistered(fnRegistered),
		server.WithHandlers(httpHandlers),

		server.WithErrHandler(app.HTTPManager().MiddlewareErrorListener()),
		server.WithTlsCert(cfg.ServerConfig.TLSCert),
		server.WithTlsKey(cfg.ServerConfig.TLSKey),
		server.WithMaxRequest(cfg.ServerConfig.MaxRequest),
		server.WithBodyLimit(cfg.ServerConfig.BodyLimit),
		server.WithExpiration(cfg.ServerConfig.Expiration),
		server.WithReadTimeout(cfg.ServerConfig.ReadTimeout),
		server.WithWriteTimeout(cfg.ServerConfig.WriteTimeout),
		server.WithIdleTimeout(cfg.ServerConfig.IdleTimeout),
		server.WithDisableStartupMessage(cfg.ServerConfig.DisableStartupMessage),
		server.WithPrefork(cfg.ServerConfig.Prefork),
		server.WithShutdown(shutdownFn),
		server.WithShutdownConnections(shutdownRealtime),
		server.WithMiddlewares(defaultMiddlewares),
	))
	if err != nil {
		return nil, fmt.Errorf("build admin server: %w", err)
	}

	assembled = true
	return srv, nil
}

type serverExtensions struct {
	staticRoutes     []StaticRoute
	middlewares      []fiber.Handler
	handlers         []server.Handler
	publicRegistered []func(*fiber.App)
	registered       []func(*fiber.App)
}

func buildServerExtensions(app *adminapp.App, builders []ExtensionBuilder) (serverExtensions, error) {
	var result serverExtensions
	for _, builder := range builders {
		ext, err := builder(app)
		if err != nil {
			return serverExtensions{}, fmt.Errorf("failed to build extension: %w", err)
		}
		if ext == nil {
			continue
		}

		if routes, ok := ext.(StaticRoutesExtension); ok {
			result.staticRoutes = append(result.staticRoutes, routes.StaticRoutes()...)
		}
		if middlewares, ok := ext.(MiddlewaresExtension); ok {
			result.middlewares = append(result.middlewares, middlewares.Middlewares()...)
		}
		if handlers, ok := ext.(HandlerExtension); ok {
			result.handlers = append(result.handlers, handlers.Handlers()...)
		}
		if register, ok := ext.(RegisterExtension); ok {
			result.registered = append(result.registered, register.Register)
		}
		if register, ok := ext.(PublicRegisterExtension); ok {
			result.publicRegistered = append(result.publicRegistered, register.RegisterPublic)
		}
	}

	return result, nil
}

func validateUploadTransports(transports []UploadTransport) error {
	for _, transport := range transports {
		if transport.Context == "" || transport.Prefix == "" || transport.Strategy == nil ||
			transport.PermissionKey.IsZero() {
			return errors.New("custom upload transport requires context, prefix, strategy, and permission key")
		}
	}

	return nil
}

type staticFilesConfig struct {
	routes         []StaticRoute
	uploadsRoot    string
	csrfCookieName string
}

func buildRegisteredHandler(
	deps Dependencies,
	app *adminapp.App,
	opts bootOptions,
	httpWS *handlers.HTTPHandler,
	uploadContextBuild uploadhost.ContextBuilder,
	staticFiles staticFilesConfig,
	publicRegistered []func(*fiber.App),
	registered []func(*fiber.App),
) func(*fiber.App) {
	return func(fiberApp *fiber.App) {
		if httpWS != nil {
			registerRealtimeRoute(fiberApp, deps.System.AdminAuthService, httpWS, staticFiles.csrfCookieName)
		}
		registerStaticRoutes(fiberApp, opts.publicFS, staticFiles.routes, publicRegistered, staticFiles.uploadsRoot,
			deps.Repos.FileRepo,
			app.Guard(integrationroles.PermissionDomainUploads, integrationroles.PermissionActionRead),
			adminmiddleware.AuthAdminMiddleware(deps.System.AdminAuthService, staticFiles.csrfCookieName),
			adminmiddleware.IsNotAuthAdminMiddleware(),
			app.HTTPManager().Middleware(),
		)
		for _, transport := range opts.uploadTransports {
			store := transport.TusStore
			if store == nil {
				store = deps.Uploads.TusStore
			}
			transportHandler := uploadhost.NewFiberUploadHandler(uploadhost.NewUploadHandler(
				deps.Uploads.Service,
				deps.Repos.FileRepo,
				store,
				deps.System.Logger,
				uploadContextBuild,
				app.ComposeFileURL,
			))
			transportHandler.RegisterStrategy(transport.Context, transport.Strategy)
			transportHandler.RegisterCMSTusRoutes(
				transport.Prefix,
				fiberApp,
				deps.System.PermissionGuard.AdminGuard(transport.PermissionKey),
			)
		}

		for _, register := range registered {
			register(fiberApp)
		}
	}
}

func registerStaticRoutes(
	fiberApp *fiber.App,
	publicFS fs.ReadFileFS,
	routes []StaticRoute,
	publicRegistered []func(*fiber.App),
	uploadsRoot string,
	fileRepo FileRepo,
	uploadReadGuard fiber.Handler,
	adminMiddlewares ...fiber.Handler,
) {
	if publicFS != nil {
		fiberApp.Use("/public", func(c fiber.Ctx) error {
			if c.Method() == fiber.MethodGet || c.Method() == fiber.MethodHead {
				defer c.Vary(fiber.HeaderAcceptEncoding)
			}
			switch c.Path() {
			case "/public/dist/js/app.js", "/public/dist/css/app.css":
				c.Set(fiber.HeaderCacheControl, "no-cache, must-revalidate")
				// Embedded files have no reliable modification date. A date-based
				// 304 would keep a previous release's fixed-name entrypoint forever.
				c.Request().Header.Del(fiber.HeaderIfModifiedSince)
				defer c.Response().Header.Del(fiber.HeaderLastModified)
			}
			return c.Next()
		})
		fiberApp.Use("/public", static.New(".", static.Config{FS: publicFS, ModifyResponse: publicAssetResponse}))
	}
	for _, route := range routes {
		fiberApp.Use(route.Path, static.New(route.Root))
	}
	for _, register := range publicRegistered {
		register(fiberApp)
	}
	for _, middleware := range adminMiddlewares {
		fiberApp.Use(middleware)
	}
	if uploadReadGuard != nil {
		fiberApp.Use("/uploads", uploadReadGuard)
	}
	if fileRepo == nil {
		return
	}
	fiberApp.Use("/uploads", func(c fiber.Ctx) error {
		admin := adminshared.GetAdminAuth(c)
		if admin == nil || admin.ID <= 0 || fileRepo == nil {
			return c.SendStatus(fiber.StatusNotFound)
		}
		relativePath, ok := requestedUploadPath(c)
		if !ok {
			return c.SendStatus(fiber.StatusNotFound)
		}
		file, err := findUploadByPath(c, fileRepo, relativePath)
		if err != nil {
			return fmt.Errorf("resolve upload path: %w", err)
		}
		if file.ID <= 0 || file.ManagerID == nil || *file.ManagerID != admin.ID ||
			!uploadintegration.Finalized(file) || path.Join(file.FolderPath, file.FileName) != relativePath {
			return c.SendStatus(fiber.StatusNotFound)
		}
		c.Set("X-Content-Type-Options", "nosniff")
		switch strings.ToLower(filepath.Ext(c.Path())) {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".bmp", ".ico":
		default:
			c.Set(fiber.HeaderContentDisposition, "attachment")
		}
		return c.Next()
	})
	fiberApp.Use("/uploads", static.New(uploadsRoot))
}

func requestedUploadPath(c fiber.Ctx) (string, bool) {
	rawPath, _, _ := strings.Cut(c.OriginalURL(), "?")
	if !strings.HasPrefix(rawPath, "/uploads/") || strings.ContainsAny(rawPath, "%\\\x00") {
		return "", false
	}
	relativePath := strings.TrimPrefix(rawPath, "/uploads/")
	if relativePath == "" || strings.HasPrefix(relativePath, "/") || strings.HasSuffix(relativePath, "/") ||
		path.Clean(relativePath) != relativePath || c.Path() != rawPath {
		return "", false
	}
	return relativePath, true
}

type uploadPathFinder interface {
	GetByPath(ctx context.Context, relativePath string) (uploadhost.File, error)
}

func findUploadByPath(ctx context.Context, repo FileRepo, relativePath string) (uploadhost.File, error) {
	if finder, ok := repo.(uploadPathFinder); ok {
		return finder.GetByPath(ctx, relativePath)
	}
	// Host repositories without path lookup are scanned in pages. They should
	// implement GetByPath for high-volume installations to avoid repeated list queries.
	const pageSize = 256
	var found uploadhost.File
	for page := 0; ; page++ {
		files, total, err := repo.List(ctx, uploadhost.ListFilters{Limit: pageSize, Offset: page * pageSize})
		if err != nil {
			return uploadhost.File{}, err
		}
		for _, file := range files {
			if path.Join(file.FolderPath, file.FileName) == relativePath {
				if found.ID != 0 {
					return uploadhost.File{}, nil
				}
				found = file
			}
		}
		if len(files) == 0 || total <= (page+1)*pageSize {
			return found, nil
		}
	}
}

func validateRolesUseCases(useCases adminroleshandler.UseCases) error {
	switch {
	case isNil(useCases.ListRoles):
		return errors.New("adminserver: RolesUseCases.ListRoles is required")
	case isNil(useCases.GetRole):
		return errors.New("adminserver: RolesUseCases.GetRole is required")
	case isNil(useCases.CreateRole):
		return errors.New("adminserver: RolesUseCases.CreateRole is required")
	case isNil(useCases.UpdateRole):
		return errors.New("adminserver: RolesUseCases.UpdateRole is required")
	case isNil(useCases.DeleteRole):
		return errors.New("adminserver: RolesUseCases.DeleteRole is required")
	case isNil(useCases.SetPermissions):
		return errors.New("adminserver: RolesUseCases.SetPermissions is required")
	case isNil(useCases.AssignAdminRoles):
		return errors.New("adminserver: RolesUseCases.AssignAdminRoles is required")
	case isNil(useCases.ListPermissions):
		return errors.New("adminserver: RolesUseCases.ListPermissions is required")
	case isNil(useCases.ListRolePermissions):
		return errors.New("adminserver: RolesUseCases.ListRolePermissions is required")
	case isNil(useCases.ListAdminRoles):
		return errors.New("adminserver: RolesUseCases.ListAdminRoles is required")
	case isNil(useCases.ListAllRoles):
		return errors.New("adminserver: RolesUseCases.ListAllRoles is required")
	default:
		return nil
	}
}

func isNil(val any) bool {
	if val == nil {
		return true
	}

	rv := reflect.ValueOf(val)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
