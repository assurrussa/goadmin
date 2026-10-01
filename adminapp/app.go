package adminapp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/assurrussa/goauth"
	logger "github.com/assurrussa/gologger"
	uploadhost "github.com/assurrussa/gouploads/host"
	eventstream "github.com/assurrussa/gowebsocket/eventstream"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"

	datagrid "github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/envs"
	"github.com/assurrussa/goadmin/services/adminservice"
)

//go:generate toolsmocks

type PermissionGuard interface {
	AdminGuard(key integrationroles.PermissionKey, opts ...integrationroles.PermissionGuardOption) fiber.Handler
}

type RolesService interface {
	IsAdoptedDetachRole(ctx context.Context, adminAuthID int64, adminID int64, roleID int64) (bool, error)
	IsSuperAdmin(ctx context.Context, adminID int64) bool
	GetRolesAdmin(ctx context.Context, adminID int64) ([]integrationroles.Role, error)
	GetPermissions(ctx context.Context, roles []integrationroles.Role) (map[int64]map[int64]integrationroles.Permission, error)
	AdminGuardCheck(
		ctx context.Context,
		adminID int64,
		key integrationroles.PermissionKey,
		opts ...integrationroles.PermissionGuardOption,
	) bool
	AdminCan(
		ctx context.Context,
		adminID int64,
		domain integrationroles.PermissionDomain,
		action integrationroles.PermissionAction,
	) bool
}

type OutboxPutter interface {
	Put(ctx context.Context, name, payload string, availableAt time.Time) (outbox.JobID, error)
}

// UserRepo is the optional repository contract consumed by the users feature.
type UserRepo interface {
	GetList(ctx context.Context, filters datagrid.Filtered) ([]integrationroles.Profile, int, error)
	GetByID(ctx context.Context, id int64) (integrationroles.Profile, error)
	Update(ctx context.Context, id int64, user integrationroles.Profile) error
	SoftDeleteByID(ctx context.Context, id int64) error
	RestoreByID(ctx context.Context, id int64) error
}

// SubjectEmailStore updates canonical subject profile data for the users feature.
type SubjectEmailStore interface {
	UpdateBasicProfile(
		ctx context.Context,
		subjectID goauth.SubjectID,
		profile goauth.BasicProfile,
	) (goauth.Account, error)
	RequestEmailChange(ctx context.Context, subjectID goauth.SubjectID, newEmail string) error
	SetSubjectStatus(
		ctx context.Context,
		subjectID goauth.SubjectID,
		status goauth.SubjectStatus,
	) (goauth.Subject, error)
}

//go:generate options-gen -out-filename=app.gen.go -from-struct=Options
type Options struct {
	httpManager     *goinertia.Inertia           `option:"mandatory" validate:"required"`
	permissionGuard PermissionGuard              `option:"mandatory" validate:"required"`
	rolesService    RolesService                 `option:"mandatory" validate:"required"`
	adminAuth       *adminservice.Service        `option:"mandatory" validate:"required"`
	session         *session.Store               `option:"mandatory" validate:"required"`
	eventStream     eventstream.EventStream      `option:"mandatory" `
	outbox          OutboxPutter                 `option:"mandatory"`
	txManager       outbox.StoragePgsqlTxManager `option:"mandatory" validate:"required"`
	logger          logger.Logger                `option:"mandatory" validate:"required"`
	env             string                       `option:"mandatory" validate:"required"`
	filesBaseURL    string                       `option:"mandatory" validate:"omitempty,url"`
	filesBucket     string                       `validate:"omitempty"`
}

type App struct {
	Options
	permissionDefinitions []integrationroles.PermissionDefinition
	commandTransaction    func(context.Context, func(context.Context) error) error
	capabilities          map[string]bool
	users                 UserRepo
	userSubjects          SubjectEmailStore
}

func Must(opts Options) *App {
	app, err := NewApp(opts)
	if err != nil {
		panic(fmt.Sprintf("failed to initialize app: %v", err))
	}

	return app
}

func NewApp(opts Options) (*App, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("validate options: %w", err)
	}

	return &App{
		Options:               opts,
		permissionDefinitions: integrationroles.ClonePermissionDefinitions(integrationroles.DefaultPermissionDefinitions().Data),
	}, nil
}

func (a *App) HTTPManager() *goinertia.Inertia {
	return a.httpManager
}

func (a *App) Session() *session.Store {
	return a.session
}

func (a *App) AdminAuth() *adminservice.Service {
	return a.adminAuth
}

func (a *App) EventStream() eventstream.EventStream {
	return a.eventStream
}

func (a *App) Outbox() OutboxPutter {
	return a.outbox
}

func (a *App) Logger() logger.Logger {
	return a.logger
}

func (a *App) TxManager() outbox.StoragePgsqlTxManager {
	return a.txManager
}

func (a *App) PermissionGuard() PermissionGuard {
	return a.permissionGuard
}

func (a *App) RolesService() RolesService {
	return a.rolesService
}

func (a *App) Env() string {
	return a.env
}

func (a *App) IsProduction() bool {
	return a.env == envs.EnvProd
}

func (a *App) ComposeFileURLFallback(primary string, fallback string) string {
	return uploadhost.ComposeFallbackFileURL(a.filesBaseURL, a.filesBucket, primary, fallback)
}

func (a *App) ComposeFileURL(raw string) string {
	return uploadhost.ComposeFileURL(a.filesBaseURL, a.filesBucket, raw)
}

func (a *App) Guard(domain integrationroles.PermissionDomain, action integrationroles.PermissionAction) fiber.Handler {
	return a.PermissionGuard().AdminGuard(integrationroles.NewPermissionKey(domain, action))
}

// SetPermissionDefinitions extends the runtime permission catalog with host-owned definitions.
func (a *App) SetPermissionDefinitions(definitions []integrationroles.PermissionDefinition) {
	a.permissionDefinitions = integrationroles.MergePermissionDefinitions(integrationroles.DefaultPermissionDefinitions().Data, definitions) //nolint:lll // required
}

// PermissionDefinitions returns a detached snapshot of the runtime permission catalog.
func (a *App) PermissionDefinitions() []integrationroles.PermissionDefinition {
	return integrationroles.ClonePermissionDefinitions(a.permissionDefinitions)
}

// SetUsersFeatureDependencies stores optional dependencies for the users feature.
func (a *App) SetUsersFeatureDependencies(users UserRepo, subjects SubjectEmailStore) {
	a.users = users
	a.userSubjects = subjects
}

// UsersFeatureRepo returns the optional users feature repository.
func (a *App) UsersFeatureRepo() UserRepo {
	return a.users
}

// UsersFeatureSubjects returns the optional users feature canonical subject store.
func (a *App) UsersFeatureSubjects() SubjectEmailStore {
	return a.userSubjects
}

func (a *App) SetCapabilities(values map[string]bool) { a.capabilities = values }
func (a *App) Enabled(key string) bool                { return a.capabilities == nil || a.capabilities[key] }

func (a *App) SetCommandTransaction(fn func(context.Context, func(context.Context) error) error) {
	a.commandTransaction = fn
}

func (a *App) InTransaction(ctx context.Context, fn func(context.Context) error) error {
	if a.commandTransaction == nil {
		return errors.New("managed admin transaction is required")
	}
	return a.commandTransaction(ctx, fn)
}
