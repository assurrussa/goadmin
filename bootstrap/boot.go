package bootstrap

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/assurrussa/goadmin/adminapp"
	"github.com/assurrussa/goadmin/infrastructure/core/menu"
	server "github.com/assurrussa/goadmin/infrastructure/fiber/server"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
	"github.com/assurrussa/goadmin/infrastructure/outbox"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
)

// Option defines a functional option for the bootstrap process.
type Option func(*bootOptions)

// WithAuthSleep sets the artificial delay for authentication (brute-force protection).
func WithAuthSleep(d time.Duration) Option {
	return func(o *bootOptions) {
		o.authSleep = d
	}
}

// WithExtension adds an extension builder to the bootstrap process.
func WithExtension(builder ExtensionBuilder) Option {
	return func(o *bootOptions) {
		o.extensionBuilders = append(o.extensionBuilders, builder)
	}
}

func WithFS(fs fs.FS) Option {
	return func(o *bootOptions) {
		o.templateFS = fs
	}
}

func WithPublicFS(fs fs.ReadFileFS) Option {
	return func(o *bootOptions) {
		o.publicFS = fs
	}
}

// WithPermissionDefinitions appends host-owned permission definitions to the admin runtime catalog.
func WithPermissionDefinitions(definitions ...integrationroles.PermissionDefinition) Option {
	return func(o *bootOptions) {
		o.permissionDefinitions = integrationroles.MergePermissionDefinitions(o.permissionDefinitions, definitions)
	}
}

// WithMenuItems appends menu items to the default admin menu.
func WithMenuItems(extra menu.Menu) Option {
	return func(o *bootOptions) {
		o.menu = o.menu.Merge(extra)
	}
}

// WithStaticRoute adds a static file route to the server.
func WithStaticRoute(path, root string) Option {
	return func(o *bootOptions) {
		o.staticRoutes = append(o.staticRoutes, StaticRoute{Path: path, Root: root})
	}
}

// WithOutboxJobs adds a job handle for outbox.
func WithOutboxJobs(jobs ...outbox.Job) Option {
	return func(o *bootOptions) {
		o.extraOutboxJobs = append(o.extraOutboxJobs, jobs...)
	}
}

// WithMiddleware adds a global middleware to the server.
func WithMiddleware(handler server.Handler) Option {
	return func(o *bootOptions) {
		o.extraMiddlewares = append(o.extraMiddlewares, handler)
	}
}

// WithUploadTransport adds an isolated, permission-guarded TUS transport.
func WithUploadTransport(transport UploadTransport) Option {
	return func(o *bootOptions) {
		o.uploadTransports = append(o.uploadTransports, transport)
	}
}

// Run initializes and returns the Admin Panel server.
// It acts as a Facade, orchestrating Inertia, AdminApp, and AdminServer setup.
func Run(
	ctx context.Context,
	cfg Config,
	deps Dependencies,
	opts ...Option,
) (*server.Server, error) {
	// 1. Default Options
	options := bootOptions{
		authSleep: 300 * time.Millisecond,
	}
	for _, o := range opts {
		o(&options)
	}

	options.menu = menu.ModuleMenu(cfg.Capabilities).Merge(options.menu)
	adminCfg := cfg.AdminConfig
	isLocalEnv := cfg.Env == "local" || cfg.Env == "dev" || cfg.Env == "development"
	rootTemplate, rootErrorTemplate := resolveInertiaRootTemplates(adminCfg.ViewsRoot, isLocalEnv)
	hotTemplate := resolveInertiaHotTemplate(adminCfg.HotFile, isLocalEnv)

	// 2. Initialize Inertia (Frontend)
	// We handle local/dev environment specifics for templates here
	var inertiaOpts []goinertia.Option
	inertiaOpts = append(inertiaOpts,
		goinertia.WithRootTemplate(rootTemplate),
		goinertia.WithRootErrorTemplate(rootErrorTemplate),
		goinertia.WithRootHotTemplate(hotTemplate),
	)

	if isLocalEnv {
		inertiaOpts = append(inertiaOpts,
			goinertia.WithFS(resolveInertiaTemplateFS(options.templateFS, isLocalEnv)),
			goinertia.WithPublicFS(nil),
		)
	} else {
		inertiaOpts = append(inertiaOpts, goinertia.WithPublicFS(options.publicFS))
		if options.templateFS != nil {
			inertiaOpts = append(inertiaOpts, goinertia.WithFS(options.templateFS))
		}
	}

	inertiaManager, err := initInertia(ctx, adminCfg, deps, options.menu, inertiaOpts...)
	if err != nil {
		return nil, fmt.Errorf("init inertia: %w", err)
	}

	if err := initOutbox(deps.Outbox, options.extraOutboxJobs...); err != nil {
		return nil, fmt.Errorf("init admin outbox: %w", err)
	}

	// 3. Initialize Admin App (Domain)
	app, err := adminapp.NewApp(adminapp.NewOptions(
		inertiaManager,
		deps.System.PermissionGuard,
		deps.Repos.RolesService,
		deps.System.AdminAuthService,
		deps.System.SessionStore,
		deps.System.EventStream,
		deps.System.Outbox,
		deps.System.TxManager,
		deps.System.Logger,
		cfg.Env,
		deps.URLs.FilesBaseURL,
		adminapp.WithFilesBucket(deps.URLs.FilesBucket),
	))
	if err != nil {
		return nil, fmt.Errorf("bootstrap: new admin app: %w", err)
	}
	app.SetCapabilities(cfg.Capabilities)
	app.SetCommandTransaction(deps.System.CommandTransaction)
	app.SetPermissionDefinitions(options.permissionDefinitions)
	app.SetUsersFeatureDependencies(deps.Repos.UserRepo, deps.System.AdminAuthService)

	// 4. Initialize Server (HTTP)
	srv, err := initServer(ctx, deps, cfg, app, options)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: init server: %w", err)
	}

	return srv, nil
}

func resolveInertiaRootTemplates(viewsRoot string, isLocalEnv bool) (string, string) { //nolint:revive // required
	if isLocalEnv {
		return filepath.Join(viewsRoot, "app.gohtml"), filepath.Join(viewsRoot, "error.gohtml")
	}

	// Production/staging use embedded templates, so ParseFS expects names
	// relative to the embedded FS root rather than disk paths.
	return "app.gohtml", "error.gohtml"
}

func resolveInertiaHotTemplate(hotFile string, isLocalEnv bool) string {
	if isLocalEnv {
		return hotFile
	}

	// Production/staging must ignore any embedded or copied Vite hot file and
	// always serve compiled admin assets.
	return ".hot-disabled"
}

func resolveInertiaTemplateFS(templateFS fs.FS, isLocalEnv bool) fs.FS {
	if isLocalEnv {
		return nil
	}

	return templateFS
}
