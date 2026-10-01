package host

import (
	"context"
	"time"

	logger "github.com/assurrussa/gologger"
	"github.com/assurrussa/gonotify/transport/notifyhub"
	uploadhost "github.com/assurrussa/gouploads/host"
	eventstream "github.com/assurrussa/gowebsocket/eventstream"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"

	"github.com/assurrussa/goadmin/adminapp"
	"github.com/assurrussa/goadmin/bootstrap"
	goadminconfig "github.com/assurrussa/goadmin/config"
	"github.com/assurrussa/goadmin/infrastructure/core/menu"
	server "github.com/assurrussa/goadmin/infrastructure/fiber/server"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
	"github.com/assurrussa/goadmin/infrastructure/notify"
	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	redis "github.com/assurrussa/goadmin/infrastructure/redis"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	csrfservice "github.com/assurrussa/goadmin/internal/csrf"
	"github.com/assurrussa/goadmin/internal/identity"
	adminloginaudit "github.com/assurrussa/goadmin/loginaudit"
)

type (
	AdminConfig           = goadminconfig.Config
	Menu                  = menu.Menu
	Section               = menu.Section
	Item                  = menu.Item
	Badge                 = menu.Badge
	Option                = bootstrap.Option
	BootstrapConfig       = bootstrap.Config
	URLConfigs            = bootstrap.URLConfigs
	PermissionKey         = integrationroles.PermissionKey
	PermissionDomain      = integrationroles.PermissionDomain
	PermissionAction      = integrationroles.PermissionAction
	PermissionDefinition  = integrationroles.PermissionDefinition
	AdminUUID             = identity.UserID
	LoginAuditRecord      = adminloginaudit.Record
	LoginAuditWriter      = adminloginaudit.Writer
	Logger                = logger.Logger
	CSRFService           = csrfservice.Service
	CSRFRequest           = csrfservice.Request
	CSRFToken             = csrfservice.Token
	CSRFClaims            = csrfservice.Claims
	Server                = server.Server
	EventStream           = eventstream.EventStream
	SessionStore          = session.Store
	RedisClient           = redis.ClientContract
	NotificationManager   = notify.NotificationManager
	NotificationConfig    = notifyhub.Config
	StoragePgsqlClient    = outbox.StoragePgsqlClient
	StoragePgsqlTxManager = outbox.StoragePgsqlTxManager
	Job                   = outbox.Job
	AfterProcessFunc      = uploadhost.AfterProcessFunc
	AfterProcessPayload   = uploadhost.AfterProcessPayload
	BatchRequest          = uploadhost.BatchRequest
	ClientError           = uploadhost.ClientError
	DeleteRequest         = uploadhost.DeleteRequest
	File                  = uploadhost.File
	FileData              = uploadhost.FileData
	FileEventAfterJob     = uploadhost.FileEventAfterJob
	FileRepo              = uploadhost.FileRepo
	FileUploadConfig      = uploadhost.FileUploadConfig
	ListFilters           = uploadhost.ListFilters
	ObjectID              = uploadhost.ObjectID
	ObjectType            = uploadhost.ObjectType
	ReaderRequest         = uploadhost.ReaderRequest
	ReaderUploadInput     = uploadhost.ReaderUploadInput
	SingleRequest         = uploadhost.SingleRequest
	TaskUploader          = uploadhost.TaskUploader
	TusStore              = uploadhost.TusStore
	AfterProcessService   = uploadhost.AfterProcessService
	UploadedFile          = uploadhost.UploadedFile
	UserID                = uploadhost.UserID
	UploadContext         = uploadhost.UploadContext
	UploadStrategy        = uploadhost.UploadStrategy
	UploadTransport       = bootstrap.UploadTransport
)

// WithUploadTransport mounts a narrow permission-guarded TUS surface on the
// embedded admin server. The transport may use its own resumable-upload store.
func WithUploadTransport(transport UploadTransport) Option {
	return bootstrap.WithUploadTransport(transport)
}

// OutboxPutter is the host-facing contract for enqueueing admin side effects.
type OutboxPutter interface {
	Put(ctx context.Context, name, payload string, availableAt time.Time) (outbox.JobID, error)
}

// PermissionGuard is the host-facing permission guard contract used by custom features.
type PermissionGuard interface {
	AdminGuard(key PermissionKey, opts ...integrationroles.PermissionGuardOption) fiber.Handler
}

// RolesService is the host-facing role service contract used by custom features and breadcrumb gating.
type RolesService interface {
	IsSuperAdmin(ctx context.Context, adminID int64) bool
	GetRolesAdmin(ctx context.Context, adminID int64) ([]integrationroles.Role, error)
	GetPermissions(ctx context.Context, roles []integrationroles.Role) (map[int64]map[int64]integrationroles.Permission, error)
	AdminGuardCheck(ctx context.Context, adminID int64, key PermissionKey, opts ...integrationroles.PermissionGuardOption) bool
	AdminCan(ctx context.Context, adminID int64, domain PermissionDomain, action PermissionAction) bool
}

// App wraps the internal admin runtime so host-mounted features do not import unstable adminapp packages.
type App struct {
	inner *adminapp.App
}

// WrapApp exposes the stable host-facing app wrapper for extension builders.
func WrapApp(app *adminapp.App) *App {
	if app == nil {
		return nil
	}

	return &App{inner: app}
}

// Unwrap returns the internal admin app for goadmin-owned internals only.
func (a *App) Unwrap() *adminapp.App {
	if a == nil {
		return nil
	}

	return a.inner
}

// Render renders an Inertia page.
func (a *App) Render(c fiber.Ctx, page string, props map[string]any) error {
	return a.inner.HTTPManager().Render(c, page, props)
}

// WithProp stores a shared prop for the current request.
func (a *App) WithProp(c fiber.Ctx, key string, value any) {
	a.inner.HTTPManager().WithProp(c, key, value)
}

// WithValidationErrors exposes validation errors for the current request.
func (a *App) WithValidationErrors(c fiber.Ctx, errors goinertia.ValidationErrors) {
	a.inner.HTTPManager().WithValidationErrors(c, errors)
}

// WithFlashSuccess stores a success flash message.
func (a *App) WithFlashSuccess(c fiber.Ctx, message string) {
	a.inner.HTTPManager().WithFlashSuccess(c, message)
}

// WithFlashWarning stores a warning flash message.
func (a *App) WithFlashWarning(c fiber.Ctx, message string) {
	a.inner.HTTPManager().WithFlashWarning(c, message)
}

// WithFlashError stores an error flash message.
func (a *App) WithFlashError(c fiber.Ctx, message string) {
	a.inner.HTTPManager().WithFlashError(c, message)
}

// WithFlashInfo stores an info flash message.
func (a *App) WithFlashInfo(c fiber.Ctx, message string) {
	a.inner.HTTPManager().WithFlashInfo(c, message)
}

// WithFlashOld stores old form data for the next request.
func (a *App) WithFlashOld(c fiber.Ctx, data map[string]any) {
	a.inner.HTTPManager().WithFlashOld(c, data)
}

// Redirect performs an Inertia redirect.
func (a *App) Redirect(c fiber.Ctx, url string) error {
	return a.inner.HTTPManager().Redirect(c, url)
}

// RedirectBack redirects to the previous location.
func (a *App) RedirectBack(c fiber.Ctx) error {
	return a.inner.HTTPManager().RedirectBack(c)
}

// RedirectBackWithValidationErrors redirects back and persists validation errors.
func (a *App) RedirectBackWithValidationErrors(c fiber.Ctx, errors goinertia.ValidationErrors) error {
	return a.inner.HTTPManager().RedirectBackWithValidationErrors(c, errors)
}

// Logger returns the app logger.
func (a *App) Logger() logger.Logger {
	return a.inner.Logger()
}

// TxManager returns the transaction manager used by the admin runtime.
func (a *App) TxManager() outbox.StoragePgsqlTxManager {
	return a.inner.TxManager()
}

// Outbox returns the outbox putter used by the admin runtime.
func (a *App) Outbox() OutboxPutter {
	return a.inner.Outbox()
}

// ComposeFileURL composes a file URL using the admin runtime's configured file URL rules.
func (a *App) ComposeFileURL(raw string) string {
	return a.inner.ComposeFileURL(raw)
}

// ComposeFileURLFallback composes a file URL with a fallback path.
func (a *App) ComposeFileURLFallback(primary, fallback string) string {
	return a.inner.ComposeFileURLFallback(primary, fallback)
}

// Guard returns a permission-guarded middleware for the provided action.
func (a *App) Guard(domain PermissionDomain, action PermissionAction) fiber.Handler {
	return a.inner.Guard(domain, action)
}

// PermissionDefinitions returns the current runtime permission catalog.
func (a *App) PermissionDefinitions() []PermissionDefinition {
	return a.inner.PermissionDefinitions()
}

// Breadcrumb represents one item in the breadcrumb trail.
type Breadcrumb struct {
	Name string `json:"name"`
	Href string `json:"href,omitempty"`
}

// ExtensionBuilder builds a feature extension against the stable host app wrapper.
type ExtensionBuilder func(app *App) (*Extension, error)

// ServerHandler is the stable handler contract accepted by host extensions.
type ServerHandler = server.Handler

const (
	PermissionDomainRoles       = integrationroles.PermissionDomainRoles
	PermissionDomainPermissions = integrationroles.PermissionDomainPermissions
	PermissionDomainAdmins      = integrationroles.PermissionDomainAdmins
	PermissionDomainDashboard   = integrationroles.PermissionDomainDashboard
	PermissionDomainOperations  = integrationroles.PermissionDomainOperations
	PermissionDomainUsers       = integrationroles.PermissionDomainUsers
	PermissionDomainUploads     = integrationroles.PermissionDomainUploads
	PermissionDomainQueues      = integrationroles.PermissionDomainQueues
)

const (
	PermissionActionRead   = integrationroles.PermissionActionRead
	PermissionActionCreate = integrationroles.PermissionActionCreate
	PermissionActionUpdate = integrationroles.PermissionActionUpdate
	PermissionActionDelete = integrationroles.PermissionActionDelete
	PermissionActionAssign = integrationroles.PermissionActionAssign
	PermissionActionSync   = integrationroles.PermissionActionSync
)

// NewPermissionKey creates a host-facing permission key without importing internal shared packages.
func NewPermissionKey(domain PermissionDomain, action PermissionAction) PermissionKey {
	return integrationroles.NewPermissionKey(domain, action)
}

// NewPermissionDefinition creates a host-facing permission definition.
func NewPermissionDefinition(key PermissionKey, description string) PermissionDefinition {
	return PermissionDefinition{
		Key:         key,
		Description: description,
	}
}

// CorePermissionDefinitions returns the built-in reusable admin permission catalog.
func CorePermissionDefinitions() []PermissionDefinition {
	return integrationroles.ClonePermissionDefinitions(integrationroles.DefaultPermissionDefinitions().Data)
}
