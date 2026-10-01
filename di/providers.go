package di

import (
	"context"

	logger "github.com/assurrussa/gologger"
	uploadhost "github.com/assurrussa/gouploads/host"
	eventstream "github.com/assurrussa/gowebsocket/eventstream"
	inmemeventstream "github.com/assurrussa/gowebsocket/eventstream/inmem"
	"github.com/gofiber/fiber/v3/middleware/session"

	"github.com/assurrussa/goadmin/adminapp"
	"github.com/assurrussa/goadmin/bootstrap"
	jobsfailedadapterrepo "github.com/assurrussa/goadmin/domain/outbox/repositories/jobsfailedrepo"
	jobsadapterrepo "github.com/assurrussa/goadmin/domain/outbox/repositories/jobsrepo"
	adminhandlersroles "github.com/assurrussa/goadmin/http/handlers/roles"
	server "github.com/assurrussa/goadmin/infrastructure/fiber/server"
	"github.com/assurrussa/goadmin/infrastructure/notify"
	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminactionauditrepo"
	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminnotificationrepo"
	adminrepo "github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminrepo"
	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/userrepo"
	redis "github.com/assurrussa/goadmin/infrastructure/redis"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	csrfservice "github.com/assurrussa/goadmin/internal/csrf"
	"github.com/assurrussa/goadmin/internal/firstadminsetup"
	adminoutbox "github.com/assurrussa/goadmin/outbox"
	adminnotificationsjob "github.com/assurrussa/goadmin/outbox/notifications"
	adminpreviewattach "github.com/assurrussa/goadmin/outbox/preview_attach"
	adminpreviewdetach "github.com/assurrussa/goadmin/outbox/preview_detach"
	"github.com/assurrussa/goadmin/services/adminservice"
)

//nolint:revive // DI wiring helper accepts many deps by design.
func provideSystemDependencies(
	lg logger.Logger,
	permissionGuard adminapp.PermissionGuard,
	csrf *csrfservice.Service,
	eventStream eventstream.EventStream,
	outbox adminapp.OutboxPutter,
	txManager outbox.StoragePgsqlTxManager,
	sessionStore *session.Store,
	adminAuthService *adminservice.Service,
	adminRepo *adminrepo.Repo,
	adminNotificationRepo *adminnotificationrepo.Repo,
	adminActionAuditRepo *adminactionauditrepo.Repo,
	firstAdminSetup *firstadminsetup.Service,
) bootstrap.SystemDependencies {
	return bootstrap.SystemDependencies{
		Logger:                lg,
		PermissionGuard:       permissionGuard,
		CSRF:                  csrf,
		EventStream:           eventStream,
		Outbox:                outbox,
		TxManager:             txManager,
		SessionStore:          sessionStore,
		AdminAuthService:      adminAuthService,
		AdminRepo:             adminRepo,
		AdminNotificationRepo: adminNotificationRepo,
		AdminActionAudit:      adminActionAuditRepo,
		FirstAdminSetup:       firstAdminSetup,
	}
}

func provideOutboxDependencies(
	outboxSvc *outbox.Service,
	adminPreviewAttach *adminpreviewattach.Job,
	adminPreviewDetach *adminpreviewdetach.Job,
	adminNotifyJob *adminnotificationsjob.Job,
) bootstrap.OutboxDependencies {
	return bootstrap.OutboxDependencies{
		Service:            outboxSvc,
		AdminPreviewAttach: adminPreviewAttach,
		AdminPreviewDetach: adminPreviewDetach,
		AdminNotifyJob:     adminNotifyJob,
	}
}

func provideAppRepositories(
	fileRepo bootstrap.FileRepo,
	fileLoader bootstrap.FileLoader,
	userRepo bootstrap.UserRepo,
	rolesService adminapp.RolesService,
	rolesManager bootstrap.RolesService,
	rolesRepo adminhandlersroles.UseCaseRepository,
	jobsRepo bootstrap.JobsRepo,
	jobsFailedRepo bootstrap.JobsFailedRepo,
) bootstrap.AppRepositories {
	return bootstrap.AppRepositories{
		FileRepo:       fileRepo,
		FileLoader:     fileLoader,
		UserRepo:       userRepo,
		RolesService:   rolesService,
		RolesManager:   rolesManager,
		RolesRepo:      rolesRepo,
		JobsRepo:       jobsRepo,
		JobsFailedRepo: jobsFailedRepo,
	}
}

func provideUploadDependencies(
	service uploadhost.TaskUploader,
	tusStore uploadhost.TusStore,
) bootstrap.UploadDependencies {
	return bootstrap.UploadDependencies{
		Service:  service,
		TusStore: tusStore,
	}
}

func provideBootstrapDependencies(
	system bootstrap.SystemDependencies,
	outbox bootstrap.OutboxDependencies,
	repos bootstrap.AppRepositories,
	uploads bootstrap.UploadDependencies,
	urls bootstrap.URLConfigs,
) bootstrap.Dependencies {
	return bootstrap.Dependencies{
		System:  system,
		Outbox:  outbox,
		Repos:   repos,
		Uploads: uploads,
		URLs:    urls,
	}
}

func provideAdminServer(
	ctx context.Context,
	cfg bootstrap.Config,
	deps bootstrap.Dependencies,
	opts []bootstrap.Option,
) (*server.Server, error) {
	return bootstrap.Run(ctx, cfg, deps, opts...)
}

func provideAdminPreviewAttach(
	evJob *uploadhost.AfterProcessService,
	adminAuthService *adminservice.Service,
	adminRepo *adminrepo.Repo,
	logger logger.Logger,
) (*adminpreviewattach.Job, error) {
	return adminpreviewattach.New(adminpreviewattach.NewOptions(
		evJob,
		adminRepo,
		adminAuthService,
		logger,
	))
}

func provideAdminPreviewDetach(
	evJob *uploadhost.AfterProcessService,
	adminAuthService *adminservice.Service,
	adminRepo *adminrepo.Repo,
	logger logger.Logger,
) (*adminpreviewdetach.Job, error) {
	return adminpreviewdetach.New(adminpreviewdetach.NewOptions(
		evJob,
		adminRepo,
		adminAuthService,
		logger,
	))
}

func provideAdminNotificationJob(
	notificationsRepo *adminnotificationrepo.Repo,
	notifier notify.NotificationManager,
	logger logger.Logger,
) (*adminnotificationsjob.Job, error) {
	return adminnotificationsjob.New(adminnotificationsjob.NewOptions(
		notificationsRepo,
		notifier,
		logger,
	))
}

func provideAdminEventStream(svc *inmemeventstream.Service) eventstream.EventStream {
	return svc
}

func provideOutbox(svc *outbox.Service) adminoutbox.Register {
	return svc
}

func provideAdminOutboxPutter(svc *outbox.Service) adminapp.OutboxPutter {
	return svc
}

func provideAdminRolesManager(svc *integrationroles.Service) bootstrap.RolesService {
	return svc
}

func provideAdminRolesUseCaseRepo(svc *integrationroles.Service) adminhandlersroles.UseCaseRepository {
	return svc
}

func provideAdminUserRepo(svc *userrepo.Repo) bootstrap.UserRepo {
	return svc
}

func provideAdminFileLoader(svc *uploadhost.FileLoader) bootstrap.FileLoader {
	return svc
}

func provideAdminFileRepo(svc *uploadhost.FileRepo) bootstrap.FileRepo {
	return svc
}

func provideAdminRepo(db outbox.StoragePgsqlClient, tx outbox.StoragePgsqlTxManager) *adminrepo.Repo {
	return adminrepo.Must(adminrepo.NewOptions(db, tx))
}

func provideAdminNotificationRepo(db outbox.StoragePgsqlClient, tx outbox.StoragePgsqlTxManager) *adminnotificationrepo.Repo {
	return adminnotificationrepo.Must(adminnotificationrepo.NewOptions(db, tx))
}

func provideAdminActionAuditRepo(db outbox.StoragePgsqlClient) *adminactionauditrepo.Repo {
	return adminactionauditrepo.Must(adminactionauditrepo.NewOptions(db))
}

func provideFirstAdminSetup(
	db outbox.StoragePgsqlClient,
	tx outbox.StoragePgsqlTxManager,
) (*firstadminsetup.Service, error) {
	store, err := firstadminsetup.NewPostgresStore(db, tx)
	if err != nil {
		return nil, err
	}

	return firstadminsetup.New(store)
}

func provideAdminAuthService(
	store *session.Store,
	rolesService adminapp.RolesService,
	authAdapter *integrationroles.Adapter,
	admins *adminrepo.Repo,
	pool redis.ClientContract,
	cfg bootstrap.Config,
) (*adminservice.Service, error) {
	states, err := authAdapter.BrowserState(cfg.AdminConfig.CanonicalSessionBackend, pool)
	if err != nil {
		return nil, err
	}
	return adminservice.NewService(
		store,
		rolesService,
		adminservice.WithRuntime(authAdapter.Runtime(), admins),
		adminservice.WithBrowserState(states),
	), nil
}

func provideAdminJobsRepo(db outbox.StoragePgsqlClient) *jobsadapterrepo.Repo {
	jobsRepo := outbox.NewPgsqlJobsRepo(db)
	return jobsadapterrepo.Must(jobsadapterrepo.NewOptions(db, jobsRepo))
}

func provideAdminJobsFailedRepo(db outbox.StoragePgsqlClient) *jobsfailedadapterrepo.Repo {
	jobsFailedRepo := outbox.NewPgsqlJobsFailedRepo(db)
	return jobsfailedadapterrepo.Must(jobsfailedadapterrepo.NewOptions(db, jobsFailedRepo))
}
