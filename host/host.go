package host

import (
	"io/fs"
	"path/filepath"
	"slices"
	"time"

	"github.com/assurrussa/goadmin/adminapp"
	"github.com/assurrussa/goadmin/bootstrap"
	goadminconfig "github.com/assurrussa/goadmin/config"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
)

const (
	DefaultPublicRoot     = "publicstaticdata"
	DefaultStaticDataRoot = "publicdata"
	DefaultViewsRoot      = "views"
	DefaultHotFileName    = "hot"
	DefaultAuthSleep      = 300 * time.Millisecond
)

const (
	EnvAppAdminDomain                  = "APP_ADMIN_DOMAIN"
	EnvAppAdminDomainURL               = "APP_ADMIN_DOMAIN_URL"
	EnvAppAdminCSRFTokenName           = "APP_ADMIN_CSRF_TOKEN_NAME" //nolint:gosec // env var name, not a secret
	EnvAppAdminCSRFTokenTTL            = "APP_ADMIN_CSRF_TOKEN_TTL"  //nolint:gosec // env var name, not a secret
	EnvAppAdminSessionInactiveTTL      = "APP_ADMIN_SESSION_INACTIVE_TTL"
	EnvAppAdminSessionTTL              = "APP_ADMIN_SESSION_TTL"
	EnvAppAdminSessionName             = "APP_ADMIN_SESSION_NAME"
	EnvAppAdminCanonicalSessionBackend = "APP_ADMIN_CANONICAL_SESSION_BACKEND"
	EnvServerAdminAddr                 = "SERVER_ADMIN_ADDR"
	EnvServerAdminBasicAuth            = "SERVER_ADMIN_BASIC_AUTH"
	EnvGoAdminPublicRoot               = "GO_ADMIN_PUBLIC_ROOT"
	EnvGoAdminHotFile                  = "GO_ADMIN_HOT_FILE"
	EnvGoAdminViewsRoot                = "GO_ADMIN_VIEWS_ROOT"
	EnvViteAdminExtRoot                = "VITE_ADMIN_EXT_ROOT"
	EnvViteAdminExtensionManifests     = "VITE_ADMIN_EXTENSION_MANIFESTS"
	EnvViteAdminCSRFCookieName         = "VITE_ADMIN_CSRF_COOKIE_NAME"
	EnvViteAdminPublicRoot             = "VITE_ADMIN_PUBLIC_ROOT"
	EnvViteAdminTSConfigPath           = "VITE_ADMIN_TSCONFIG_PATH"
)

var runtimeEnvNames = []string{
	EnvAppAdminDomain,
	EnvAppAdminDomainURL,
	EnvAppAdminCSRFTokenName,
	EnvAppAdminCSRFTokenTTL,
	EnvAppAdminSessionInactiveTTL,
	EnvAppAdminSessionTTL,
	EnvAppAdminSessionName,
	EnvAppAdminCanonicalSessionBackend,
	EnvServerAdminAddr,
	EnvServerAdminBasicAuth,
	EnvGoAdminPublicRoot,
	EnvGoAdminHotFile,
	EnvGoAdminViewsRoot,
}

var buildEnvNames = []string{
	EnvViteAdminExtRoot,
	EnvViteAdminExtensionManifests,
	EnvViteAdminCSRFCookieName,
	EnvViteAdminPublicRoot,
	EnvViteAdminTSConfigPath,
}

// RuntimeEnvNames returns the backend/admin runtime env contract for the host.
func RuntimeEnvNames() []string {
	return slices.Clone(runtimeEnvNames)
}

// BuildEnvNames returns the admin asset-build env contract for the host.
func BuildEnvNames() []string {
	return slices.Clone(buildEnvNames)
}

// PathDefaults declares the filesystem defaults for a concrete host project.
type PathDefaults struct {
	PublicRoot     string
	StaticDataRoot string
	ViewsRoot      string
}

// AdminConfigInput is the host-owned input mapped into goadmin config.
type AdminConfigInput struct {
	Env                     string
	BaseDomain              string
	BaseDomainURL           string
	Domain                  string
	DomainURL               string
	Addr                    string
	BasicAuth               []string
	CSRFTokenName           string
	CSRFTokenTTL            time.Duration
	SessionInactiveTTL      time.Duration
	SessionTTL              time.Duration
	SessionName             string
	CanonicalSessionBackend string
	PublicRoot              string
	StaticDataRoot          string
	HotFile                 string
	ViewsRoot               string
}

// ServerConfigInput is the host-owned transport/server input for the admin runtime.
type ServerConfigInput struct {
	Addr                  string
	AllowOrigins          []string
	MaxRequest            int
	BodyLimit             int
	Expiration            time.Duration
	ReadTimeout           time.Duration
	WriteTimeout          time.Duration
	IdleTimeout           time.Duration
	DisableStartupMessage bool
	Prefork               bool
	TLSCert               string
	TLSKey                string
}

// BootstrapConfigInput combines admin and server settings for bootstrap.Config.
type BootstrapConfigInput struct {
	Admin        AdminConfigInput
	PathDefaults PathDefaults
	Server       ServerConfigInput
}

// URLConfigInput is the minimal host-facing URL contract needed by bootstrap.
type URLConfigInput struct {
	FilesBaseURL string
	FilesBucket  string
}

// FeatureRegistry is the narrow host-facing registry contract consumed here.
type FeatureRegistry interface {
	Menu() Menu
	PermissionDefinitions() []PermissionDefinition
	ExtensionBuilders(siteMenu Menu) []ExtensionBuilder
}

// BootstrapOptionsInput maps host assets and registry wiring into bootstrap options.
type BootstrapOptionsInput struct {
	Env        string
	Registry   FeatureRegistry
	TemplateFS fs.FS
	PublicFS   fs.ReadFileFS
	AuthSleep  time.Duration
}

// BootstrapOptionsPlan is an inspectable representation of bootstrap options.
type BootstrapOptionsPlan struct {
	AuthSleep             time.Duration
	Menu                  Menu
	PermissionDefinitions []PermissionDefinition
	TemplateFS            fs.FS
	PublicFS              fs.ReadFileFS
	IncludePublicFS       bool
	ExtensionBuilders     []ExtensionBuilder
}

// Options converts the plan into goadmin bootstrap options.
func (p BootstrapOptionsPlan) Options() []Option {
	opts := []Option{
		bootstrap.WithAuthSleep(p.AuthSleep),
		bootstrap.WithMenuItems(p.Menu),
	}
	if len(p.PermissionDefinitions) > 0 {
		opts = append(opts, bootstrap.WithPermissionDefinitions(p.PermissionDefinitions...))
	}

	if p.TemplateFS != nil {
		opts = append(opts, bootstrap.WithFS(p.TemplateFS))
	}
	if p.IncludePublicFS && p.PublicFS != nil {
		opts = append(opts, bootstrap.WithPublicFS(p.PublicFS))
	}
	for _, builder := range p.ExtensionBuilders {
		if builder == nil {
			continue
		}
		hostBuilder := builder
		opts = append(opts, bootstrap.WithExtension(func(app *adminapp.App) (bootstrap.Extension, error) {
			return hostBuilder(WrapApp(app))
		}))
	}

	return opts
}

// BuildConfig resolves host defaults into the goadmin runtime config.
func BuildConfig(input AdminConfigInput, defaults PathDefaults) AdminConfig {
	defaults = normalizePathDefaults(defaults)

	publicRoot := firstNonEmpty(input.PublicRoot, defaults.PublicRoot)
	staticDataRoot := firstNonEmpty(input.StaticDataRoot, defaults.StaticDataRoot)
	hotFile := firstNonEmpty(input.HotFile, filepath.Join(publicRoot, DefaultHotFileName))
	viewsRoot := firstNonEmpty(input.ViewsRoot, defaults.ViewsRoot)

	return AdminConfig{
		Env:                     input.Env,
		BaseDomain:              input.BaseDomain,
		BaseDomainURL:           input.BaseDomainURL,
		Domain:                  input.Domain,
		DomainURL:               input.DomainURL,
		Addr:                    input.Addr,
		BasicAuth:               slices.Clone(input.BasicAuth),
		CSRFTokenName:           input.CSRFTokenName,
		CSRFTokenTTL:            input.CSRFTokenTTL,
		SessionInactiveTTL:      input.SessionInactiveTTL,
		SessionTTL:              input.SessionTTL,
		SessionName:             input.SessionName,
		CanonicalSessionBackend: firstNonEmpty(input.CanonicalSessionBackend, goadminconfig.AdminCanonicalSessionBackendPGSQL),
		PublicRoot:              publicRoot,
		StaticDataRoot:          staticDataRoot,
		HotFile:                 hotFile,
		ViewsRoot:               viewsRoot,
	}
}

// BuildBootstrapConfig derives the reusable bootstrap.Config from host inputs.
func BuildBootstrapConfig(input BootstrapConfigInput) BootstrapConfig {
	adminCfg := BuildConfig(input.Admin, input.PathDefaults)
	addr := firstNonEmpty(input.Server.Addr, adminCfg.Addr)

	return bootstrap.Config{
		Env:         adminCfg.Env,
		AdminConfig: adminCfg,
		ServerConfig: bootstrap.ServerConfig{
			Addr:                  addr,
			AllowOrigins:          slices.Clone(input.Server.AllowOrigins),
			MaxRequest:            input.Server.MaxRequest,
			BodyLimit:             input.Server.BodyLimit,
			Expiration:            input.Server.Expiration,
			ReadTimeout:           input.Server.ReadTimeout,
			WriteTimeout:          input.Server.WriteTimeout,
			IdleTimeout:           input.Server.IdleTimeout,
			DisableStartupMessage: input.Server.DisableStartupMessage,
			Prefork:               input.Server.Prefork,
			TLSCert:               input.Server.TLSCert,
			TLSKey:                input.Server.TLSKey,
		},
	}
}

// BuildURLConfigs maps the minimal URL contract into bootstrap.URLConfigs.
func BuildURLConfigs(input URLConfigInput) URLConfigs {
	return URLConfigs{
		FilesBaseURL: input.FilesBaseURL,
		FilesBucket:  input.FilesBucket,
	}
}

// BuildBootstrapOptionsPlan resolves registry/assets into an inspectable plan.
func BuildBootstrapOptionsPlan(input BootstrapOptionsInput) BootstrapOptionsPlan {
	authSleep := input.AuthSleep
	if authSleep <= 0 {
		authSleep = DefaultAuthSleep
	}

	var adminMenu Menu
	var builders []ExtensionBuilder
	var permissionDefinitions []PermissionDefinition
	if input.Registry != nil {
		adminMenu = input.Registry.Menu()
		permissionDefinitions = input.Registry.PermissionDefinitions()
		builders = input.Registry.ExtensionBuilders(adminMenu)
	}

	return BootstrapOptionsPlan{
		AuthSleep:             authSleep,
		Menu:                  adminMenu,
		PermissionDefinitions: integrationroles.ClonePermissionDefinitions(permissionDefinitions),
		TemplateFS:            input.TemplateFS,
		PublicFS:              input.PublicFS,
		IncludePublicFS:       !IsLocalEnv(input.Env),
		ExtensionBuilders:     slices.Clone(builders),
	}
}

// BuildBootstrapOptions converts the resolved plan into bootstrap options.
func BuildBootstrapOptions(input BootstrapOptionsInput) []Option {
	return BuildBootstrapOptionsPlan(input).Options()
}

// IsLocalEnv reports whether the host should behave like a local/dev runtime.
func IsLocalEnv(env string) bool {
	switch env {
	case "local", "dev", "development":
		return true
	default:
		return false
	}
}

func normalizePathDefaults(defaults PathDefaults) PathDefaults {
	return PathDefaults{
		PublicRoot:     firstNonEmpty(defaults.PublicRoot, DefaultPublicRoot),
		StaticDataRoot: firstNonEmpty(defaults.StaticDataRoot, DefaultStaticDataRoot),
		ViewsRoot:      firstNonEmpty(defaults.ViewsRoot, DefaultViewsRoot),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}
