package bootstrap

import (
	"context"
	"io/fs"
	"time"

	"github.com/assurrussa/goauth"
	logger "github.com/assurrussa/gologger"
	uploadhost "github.com/assurrussa/gouploads/host"
	eventstream "github.com/assurrussa/gowebsocket/eventstream"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"

	"github.com/assurrussa/goadmin/adminapp"
	"github.com/assurrussa/goadmin/config"
	adminhandlersroles "github.com/assurrussa/goadmin/http/handlers/roles"
	datagrid "github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	"github.com/assurrussa/goadmin/infrastructure/core/menu"
	server "github.com/assurrussa/goadmin/infrastructure/fiber/server"
	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	adminactionauditrepo "github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminactionauditrepo"
	adminnotificationrepo "github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminnotificationrepo"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	csrfservice "github.com/assurrussa/goadmin/internal/csrf"
	identity "github.com/assurrussa/goadmin/internal/identity"
	adminloginaudit "github.com/assurrussa/goadmin/loginaudit"
	adminmodels "github.com/assurrussa/goadmin/models"
	adminoutbox "github.com/assurrussa/goadmin/outbox"
	adminnotificationsjob "github.com/assurrussa/goadmin/outbox/notifications"
	adminpreviewattach "github.com/assurrussa/goadmin/outbox/preview_attach"
	adminpreviewdetach "github.com/assurrussa/goadmin/outbox/preview_detach"
	"github.com/assurrussa/goadmin/services/adminservice"
)

// Config объединяет все настройки.
type Config struct {
	Env                    string
	Capabilities           map[string]bool
	AdminConfig            config.Config
	ServerConfig           ServerConfig
	AuthRuntime            goauth.Config
	AuthRuntimeAutoMigrate bool
}

type ServerConfig struct {
	Addr         string
	AllowOrigins []string
	MaxRequest   int
	BodyLimit    int
	Expiration   time.Duration
	// Zero selects bounded defaults: 10s read/write and 120s idle.
	ReadTimeout           time.Duration
	WriteTimeout          time.Duration
	IdleTimeout           time.Duration
	DisableStartupMessage bool
	Prefork               bool
	TLSCert               string
	TLSKey                string
}

type FileRepo interface {
	GetByID(ctx context.Context, id int64) (uploadhost.File, error)
	Update(ctx context.Context, id int64, file uploadhost.File) error
	List(ctx context.Context, filters uploadhost.ListFilters) ([]uploadhost.File, int, error)
}

type FileLoader interface {
	LoadPreview(ctx context.Context, fileID int64) (*uploadhost.File, error)
	LoadPreviewURL(ctx context.Context, fileID int64) (string, error)
}

type UserRepo interface {
	GetList(ctx context.Context, filters datagrid.Filtered) ([]integrationroles.Profile, int, error)
	GetByID(ctx context.Context, id int64) (integrationroles.Profile, error)
	Update(ctx context.Context, id int64, user integrationroles.Profile) error
	SoftDeleteByID(ctx context.Context, id int64) error
	RestoreByID(ctx context.Context, id int64) error
}

type RolesService interface {
	adminhandlersroles.UseCaseService
	EnsurePermissions(ctx context.Context, inputs []integrationroles.CreatePermissionInput) error
	CreatePermissions(ctx context.Context, inputs []integrationroles.CreatePermissionInput) ([]integrationroles.Permission, error)
}

type JobsRepo interface {
	datagrid.Repository[outbox.JobModel]
	DeleteJob(ctx context.Context, jobID outbox.JobID) (int64, error)
	CountExact(ctx context.Context) (int64, error)
	Create(ctx context.Context, job outbox.JobModel) (outbox.JobID, error)
}

type JobsFailedRepo interface {
	datagrid.Repository[outbox.JobFailedModel]
	Delete(ctx context.Context, jobID outbox.JobID) (int64, error)
	GetByID(ctx context.Context, id outbox.JobID) (outbox.JobFailedModel, error)
	CountExact(ctx context.Context) (int64, error)
}

type NotificationsRepo interface {
	Create(ctx context.Context, params adminnotificationrepo.CreateParams) (int64, error)
	CountUnread(ctx context.Context, adminID int64) (int, error)
	ListByAdmin(ctx context.Context, params adminnotificationrepo.ListParams) (adminnotificationrepo.ListResult, error)
	MarkRead(ctx context.Context, adminID, notificationID int64) error
	MarkAllRead(ctx context.Context, adminID int64) (int64, error)
}

type AdminRepo interface {
	ProvisionAccount(
		ctx context.Context,
		account goauth.Account,
		publicID identity.UserID,
	) (adminmodels.Admin, error)
	UpdateAdmin(ctx context.Context, id int64, admin adminmodels.Admin) error
	GetList(ctx context.Context, filters datagrid.Filtered) ([]adminmodels.Admin, int, error)
	GetByID(ctx context.Context, id int64) (adminmodels.Admin, error)
	GetBySubjectID(ctx context.Context, subjectID integrationroles.SubjectID) (adminmodels.Admin, error)
	DeleteByID(ctx context.Context, id int64) error
	SoftDeleteByID(ctx context.Context, id int64) error
	GetByUUID(ctx context.Context, id identity.UserID) (adminmodels.Admin, error)
	UpdatePreview(ctx context.Context, admin adminmodels.Admin, fileID int64) error
}

type FirstAdminSetupTokens interface {
	Validate(ctx context.Context, token string) error
	Consume(ctx context.Context, token string) error
	StaticConfigured() bool
}

// SystemDependencies - это инфраструктурные сервисы, которые часто уже есть в Module.
type SystemDependencies struct {
	CommandTransaction func(context.Context, func(context.Context) error) error
	Logger             logger.Logger
	PermissionGuard    adminapp.PermissionGuard
	CSRF               *csrfservice.Service
	EventStream        eventstream.EventStream
	// ShutdownEventStream is set only for a stream owned by this runtime.
	ShutdownEventStream   func(context.Context) error
	Outbox                adminapp.OutboxPutter
	TxManager             outbox.StoragePgsqlTxManager
	SessionStore          *session.Store
	AdminAuthService      *adminservice.Service
	AdminRepo             AdminRepo
	AdminNotificationRepo NotificationsRepo
	AdminActionAudit      adminactionauditrepo.Writer
	FirstAdminSetup       FirstAdminSetupTokens
}

// AppRepositories - это репозитории, которые ДОЛЖЕН предоставить хост-проект.
type AppRepositories struct {
	FileRepo        FileRepo
	FileLoader      FileLoader
	UserRepo        UserRepo
	AdminLoginAudit adminloginaudit.Writer
	// Опционально, если хост-проект хочет переопределить работу с ролями
	RolesService adminapp.RolesService
	RolesManager RolesService
	RolesRepo    adminhandlersroles.UseCaseRepository

	// Опционально, для очередей
	JobsRepo       JobsRepo
	JobsFailedRepo JobsFailedRepo
}

// UploadDependencies - зависимости для загрузки файлов.
type UploadDependencies struct {
	Service    uploadhost.TaskUploader
	TusStore   uploadhost.TusStore
	Strategies map[string]uploadhost.UploadStrategy
}

// OutboxDependencies - зависимости для job.
type OutboxDependencies struct {
	Service            adminoutbox.Register
	AdminPreviewAttach *adminpreviewattach.Job
	AdminPreviewDetach *adminpreviewdetach.Job
	AdminNotifyJob     *adminnotificationsjob.Job
}

// URLConfigs - простые строковые настройки.
type URLConfigs struct {
	FilesBaseURL string
	FilesBucket  string
}

// Dependencies - общая структура, собирающая всё вместе.
type Dependencies struct {
	Capabilities map[string]bool
	System       SystemDependencies
	Repos        AppRepositories
	Uploads      UploadDependencies
	Outbox       OutboxDependencies
	URLs         URLConfigs
}

type ExtensionBuilder func(app *adminapp.App) (Extension, error)

type StaticRoute struct {
	Path string
	Root string
}

type Extension any

type RegisterExtension interface {
	Register(app *fiber.App)
}

// PublicRegisterExtension mounts host-owned public routes before the embedded
// admin authentication middleware is installed.
type PublicRegisterExtension interface {
	RegisterPublic(app *fiber.App)
}

type HandlerExtension interface {
	Handlers() []server.Handler
}

type StaticRoutesExtension interface {
	StaticRoutes() []StaticRoute
}

type MiddlewaresExtension interface {
	Middlewares() []fiber.Handler
}

// UploadTransport mounts an isolated resumable-upload surface. It intentionally
// exposes only the TUS create/resume transport, not generic completion or file
// management. TusStore may select an isolated storage backend.
type UploadTransport struct {
	Context       string
	Prefix        string
	Strategy      uploadhost.UploadStrategy
	TusStore      uploadhost.TusStore
	PermissionKey integrationroles.PermissionKey
}

type bootOptions struct {
	authSleep             time.Duration
	templateFS            fs.FS
	publicFS              fs.ReadFileFS
	permissionDefinitions []integrationroles.PermissionDefinition
	extensionBuilders     []ExtensionBuilder
	staticRoutes          []StaticRoute
	extraMiddlewares      []server.Handler
	extraOutboxJobs       []outbox.Job
	uploadTransports      []UploadTransport
	menu                  menu.Menu
}

// Enabled preserves legacy assembly only when no capability snapshot is supplied.
func (c Config) Enabled(key string) bool { return c.Capabilities == nil || c.Capabilities[key] }
