package host

import (
	"context"
	"fmt"

	"github.com/assurrussa/goauth/postgres"
	logger "github.com/assurrussa/gologger"
	uploadhost "github.com/assurrussa/gouploads/host"
	eventstream "github.com/assurrussa/gowebsocket/eventstream"
	"github.com/gofiber/fiber/v3/middleware/session"

	"github.com/assurrussa/goadmin/bootstrap"
	jobsfailedadapterrepo "github.com/assurrussa/goadmin/domain/outbox/repositories/jobsfailedrepo"
	jobsadapterrepo "github.com/assurrussa/goadmin/domain/outbox/repositories/jobsrepo"
	"github.com/assurrussa/goadmin/infrastructure/notify"
	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminactionauditrepo"
	adminnotificationrepo "github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminnotificationrepo"
	goadminuserrepo "github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/userrepo"
	redis "github.com/assurrussa/goadmin/infrastructure/redis"
	"github.com/assurrussa/goadmin/internal/admintx"
	"github.com/assurrussa/goadmin/internal/firstadminsetup"
	adminnotificationsjob "github.com/assurrussa/goadmin/outbox/notifications"
	adminpreviewattach "github.com/assurrussa/goadmin/outbox/preview_attach"
	adminpreviewdetach "github.com/assurrussa/goadmin/outbox/preview_detach"
	"github.com/assurrussa/goadmin/services/adminservice"
)

// FileRepository is the host-supplied files storage contract used by built-in admin screens.
type FileRepository interface {
	GetByID(ctx context.Context, id int64) (uploadhost.File, error)
	Update(ctx context.Context, id int64, file uploadhost.File) error
	List(ctx context.Context, filters uploadhost.ListFilters) ([]uploadhost.File, int, error)
}

// FileLoader resolves file previews for built-in admin screens.
type FileLoader interface {
	LoadPreview(ctx context.Context, fileID int64) (*uploadhost.File, error)
	LoadPreviewURL(ctx context.Context, fileID int64) (string, error)
}

// Outbox combines enqueueing and job registration for built-in admin jobs.
type Outbox interface {
	OutboxPutter
	RegisterJob(job outbox.Job) error
	MustRegisterJob(job outbox.Job)
}

// Services collects external infrastructure that the embedded admin mounts onto.
type Services struct {
	ExternalAuthority ExternalSessionAuthority
	DB                outbox.StoragePgsqlClient
	TxManager         outbox.StoragePgsqlTxManager
	Logger            logger.Logger
	CSRF              *CSRFService
	EventStream       eventstream.EventStream
	Outbox            Outbox
	SessionStore      *session.Store
	SessionRedis      redis.ClientContract
	Notifier          notify.NotificationManager
	// SubjectPermissions may be prebuilt and shared with transport-neutral
	// features such as gocms. Install builds it when omitted.
	SubjectPermissions *SubjectPermissionChecker
}

// Repositories are host-owned data services used by the embedded admin runtime.
type Repositories struct {
	FileRepo        FileRepository
	FileLoader      FileLoader
	AdminLoginAudit LoginAuditWriter
}

// Uploads groups upload services required by built-in admin file flows.
type Uploads struct {
	Service      uploadhost.TaskUploader
	TusStore     uploadhost.TusStore
	AfterProcess AfterProcessHandler
}

// assemblyInput contains the private, normalized assembly graph.
type assemblyInput struct {
	Config               BootstrapConfig
	Auth                 *AuthAdapter
	Services             Services
	Repositories         Repositories
	Uploads              Uploads
	UploadStrategies     map[string]UploadStrategy
	URLs                 URLConfigs
	Options              []Option
	ClientBundle         *ClientBundleValidation
	FirstAdminSetupToken FirstAdminSetupToken
}

// AfterProcessHandler handles upload after-process payloads for built-in admin jobs.
type AfterProcessHandler interface {
	HandleAfterProcess(ctx context.Context, payload string, fnCall AfterProcessFunc) error
}

func buildDependencies(input assemblyInput) (bootstrap.Dependencies, error) {
	enabled := input.Config.Enabled
	lg := input.Services.Logger
	if lg == nil {
		lg = logger.Discard()
	}

	adminRepoImpl := input.Auth.admins
	authAdapter := input.Auth.inner
	rolesService := authAdapter.Roles()
	rolesRepo := rolesService
	rolesGuard, mutationRolesGuard, err := buildAdminRoleGuards(
		rolesService, input.Services.SubjectPermissions, adminRepoImpl, lg,
	)
	if err != nil {
		return bootstrap.Dependencies{}, err
	}
	var adminNotificationRepoImpl bootstrap.NotificationsRepo
	if enabled("notifications") {
		adminNotificationRepoImpl, err = adminnotificationrepo.New(
			adminnotificationrepo.NewOptions(input.Services.DB, input.Services.TxManager),
		)
		if err != nil {
			return bootstrap.Dependencies{}, fmt.Errorf("host install: build admin notification repo: %w", err)
		}
	}
	adminActionAuditRepoImpl, err := adminactionauditrepo.New(adminactionauditrepo.NewOptions(input.Services.DB))
	if err != nil {
		return bootstrap.Dependencies{}, fmt.Errorf("host install: build admin action audit repo: %w", err)
	}
	var adminUsersRepo bootstrap.UserRepo
	if enabled("users") {
		adminUsersRepo, err = goadminuserrepo.New(goadminuserrepo.NewOptions(input.Services.DB, input.Services.TxManager))
		if err != nil {
			return bootstrap.Dependencies{}, fmt.Errorf("host install: build admin users repo: %w", err)
		}
	}
	firstAdminSetupStore, err := firstadminsetup.NewPostgresStore(input.Services.DB, input.Services.TxManager)
	if err != nil {
		return bootstrap.Dependencies{}, fmt.Errorf("host install: build first admin setup store: %w", err)
	}
	firstAdminSetupOptions := make([]firstadminsetup.Option, 0, 1)
	if input.FirstAdminSetupToken.configured {
		firstAdminSetupOptions = append(
			firstAdminSetupOptions,
			firstadminsetup.WithStaticTokenHash(input.FirstAdminSetupToken.hash),
		)
	}
	firstAdminSetupService, err := firstadminsetup.New(firstAdminSetupStore, firstAdminSetupOptions...)
	if err != nil {
		return bootstrap.Dependencies{}, fmt.Errorf("host install: build first admin setup service: %w", err)
	}

	states, err := authAdapter.BrowserState(input.Config.AdminConfig.CanonicalSessionBackend, input.Services.SessionRedis)
	if err != nil {
		return bootstrap.Dependencies{}, fmt.Errorf("host install: browser auth state: %w", err)
	}
	links, err := postgres.NewStore(input.Auth.Runtime().Database())
	if err != nil {
		return bootstrap.Dependencies{}, err
	}
	adminAuthService := adminservice.NewService(
		input.Services.SessionStore,
		rolesGuard,
		adminservice.WithRuntime(authAdapter.Runtime(), adminRepoImpl),
		adminservice.WithBrowserState(states),
		adminservice.WithExternalAuthority(input.Services.ExternalAuthority, links),
	)

	var jobsRepo bootstrap.JobsRepo
	var jobsFailedRepo bootstrap.JobsFailedRepo
	if enabled("queues") {
		coreJobsRepo := outbox.NewPgsqlJobsRepo(input.Services.DB)
		jobsRepo, err = jobsadapterrepo.New(jobsadapterrepo.NewOptions(input.Services.DB, coreJobsRepo))
		if err != nil {
			return bootstrap.Dependencies{}, fmt.Errorf("host install: build jobs repo: %w", err)
		}
		coreJobsFailedRepo := outbox.NewPgsqlJobsFailedRepo(input.Services.DB)
		jobsFailedRepo, err = jobsfailedadapterrepo.New(
			jobsfailedadapterrepo.NewOptions(input.Services.DB, coreJobsFailedRepo),
		)
		if err != nil {
			return bootstrap.Dependencies{}, fmt.Errorf("host install: build failed jobs repo: %w", err)
		}
	}

	moduleJobs, err := buildModuleJobs(input, adminNotificationRepoImpl, adminAuthService)
	if err != nil {
		return bootstrap.Dependencies{}, err
	}

	return bootstrap.Dependencies{
		Capabilities: input.Config.Capabilities,
		System: bootstrap.SystemDependencies{
			CommandTransaction: func(ctx context.Context, fn func(context.Context) error) error {
				return input.Auth.Runtime().InAuthTransaction(ctx, func(txCtx context.Context) error {
					executor, err := input.Auth.Runtime().SQLExecutor(txCtx)
					if err != nil {
						return err
					}
					return fn(admintx.WithExecutor(txCtx, executor, input.Services.DB.DB().Pool()))
				})
			},
			Logger:                lg,
			PermissionGuard:       rolesGuard,
			CSRF:                  input.Services.CSRF,
			EventStream:           input.Services.EventStream,
			Outbox:                input.Services.Outbox,
			TxManager:             input.Services.TxManager,
			SessionStore:          input.Services.SessionStore,
			AdminAuthService:      adminAuthService,
			AdminRepo:             adminRepoImpl,
			AdminNotificationRepo: adminNotificationRepoImpl,
			AdminActionAudit:      adminActionAuditRepoImpl,
			FirstAdminSetup:       firstAdminSetupService,
		},
		Repos: bootstrap.AppRepositories{
			FileRepo:        input.Repositories.FileRepo,
			FileLoader:      input.Repositories.FileLoader,
			UserRepo:        adminUsersRepo,
			AdminLoginAudit: input.Repositories.AdminLoginAudit,
			RolesService:    mutationRolesGuard,
			RolesManager:    rolesService,
			RolesRepo:       rolesRepo,
			JobsRepo:        jobsRepo,
			JobsFailedRepo:  jobsFailedRepo,
		},
		Uploads: bootstrap.UploadDependencies{
			Service:    input.Uploads.Service,
			TusStore:   input.Uploads.TusStore,
			Strategies: input.UploadStrategies,
		},
		Outbox: moduleJobs,
		URLs:   input.URLs,
	}, nil
}

func buildModuleJobs(input assemblyInput, notifications bootstrap.NotificationsRepo,
	auth *adminservice.Service,
) (bootstrap.OutboxDependencies, error) {
	adminRepoImpl := input.Auth.admins
	adminAuthService := auth
	adminNotificationRepoImpl := notifications
	var err error
	var adminPreviewAttach *adminpreviewattach.Job
	var adminPreviewDetach *adminpreviewdetach.Job
	if input.Config.Enabled("uploads") {
		adminPreviewAttach, err = adminpreviewattach.New(adminpreviewattach.NewOptions(
			input.Uploads.AfterProcess,
			adminRepoImpl,
			adminAuthService,
			input.Services.Logger,
		))
		if err != nil {
			return bootstrap.OutboxDependencies{}, fmt.Errorf("host install: build preview attach job: %w", err)
		}
		adminPreviewDetach, err = adminpreviewdetach.New(adminpreviewdetach.NewOptions(
			input.Uploads.AfterProcess,
			adminRepoImpl,
			adminAuthService,
			input.Services.Logger,
		))
		if err != nil {
			return bootstrap.OutboxDependencies{}, fmt.Errorf("host install: build preview detach job: %w", err)
		}
	}
	var adminNotifyJob *adminnotificationsjob.Job
	if input.Config.Enabled("notifications") {
		adminNotifyJob, err = adminnotificationsjob.New(adminnotificationsjob.NewOptions(
			adminNotificationRepoImpl,
			input.Services.Notifier,
			input.Services.Logger,
		))
		if err != nil {
			return bootstrap.OutboxDependencies{}, fmt.Errorf("host install: build notification job: %w", err)
		}
	}

	return bootstrap.OutboxDependencies{
		Service: input.Services.Outbox, AdminPreviewAttach: adminPreviewAttach,
		AdminPreviewDetach: adminPreviewDetach, AdminNotifyJob: adminNotifyJob,
	}, nil
}
