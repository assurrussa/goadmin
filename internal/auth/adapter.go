package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/postgres"
	"github.com/assurrussa/goauth/rbac"
	"github.com/jackc/pgx/v5/stdlib"
	redis "github.com/redis/go-redis/v9"

	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/internal/admintx"
	"github.com/assurrussa/goadmin/internal/auth/browserstate"
	"github.com/assurrussa/goadmin/internal/authsnapshot"
	identity "github.com/assurrussa/goadmin/internal/identity"
)

type AdminMemberships interface {
	HasAdminMembership(ctx context.Context, subjectID goauth.SubjectID) (bool, error)
	ProvisionAdminMembership(
		ctx context.Context,
		account goauth.Account,
		publicID identity.UserID,
	) (int64, error)
}

type Config struct {
	AuthorizationCache *AuthorizationCacheConfig
	Database           outbox.StoragePgsqlClient
	Runtime            goauth.Config
	Memberships        AdminMemberships
	AutoMigrate        bool
	NotificationSender goauth.NotificationSender
	NotificationWorker postgres.NotificationWorkerConfig
}

// Adapter is the single goadmin boundary around the supported goauth v0.2
// Runtime and RBAC APIs. Host DTOs are mapped in this package and never enter
// goauth's canonical models.
type Adapter struct {
	authorization *authsnapshot.Cache
	runtime       *postgres.Runtime
	roles         *RoleService
	memberships   AdminMemberships
	db            *sql.DB
	database      outbox.StoragePgsqlClient
	stateKeys     goauth.KeyRing
}

func New(config Config) (*Adapter, error) {
	if config.Database == nil || config.Database.DB() == nil || config.Database.DB().Pool() == nil {
		return nil, errors.New("goadmin auth: PostgreSQL pool is required")
	}
	if config.Memberships == nil {
		return nil, errors.New("goadmin auth: admin memberships are required")
	}
	memberships := config.Memberships
	var cached *cachedMemberships
	if config.AuthorizationCache != nil {
		if _, ok := memberships.(membershipMutationConfigurer); !ok {
			return nil, errors.New("admin authorization cache requires membership mutation hooks")
		}
		cached = &cachedMemberships{AdminMemberships: memberships, pool: config.Database.DB().Pool()}
		memberships = cached
	}
	config.Runtime.MembershipGate = adminMembershipGate(config.Runtime.MembershipGate, memberships)

	db := stdlib.OpenDBFromPool(config.Database.DB().Pool())
	runtime, err := postgres.NewRuntime(postgres.Config{
		DB:                 db,
		AutoMigrate:        config.AutoMigrate,
		Runtime:            config.Runtime,
		NotificationSender: config.NotificationSender,
		NotificationWorker: config.NotificationWorker,
	})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("goadmin auth: create Runtime: %w", err)
	}
	var authorization *authsnapshot.Cache
	if cached != nil {
		authorization, err = authsnapshot.New(context.Background(), loadAuthorization(runtime), *config.AuthorizationCache)
		if err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("initialize admin authorization cache: %w", err)
		}
		cached.cache = authorization
		cached.runtime = runtime
	}
	var rolesCache rbac.Cache
	if authorization != nil {
		rolesCache = authorizationInvalidator{cache: authorization}
	}
	roles, err := runtime.RBAC(rolesCache)
	if err != nil {
		if authorization != nil {
			authorization.Close()
		}
		_ = db.Close()
		return nil, fmt.Errorf("goadmin auth: create RBAC: %w", err)
	}

	roleService := NewRoleService(roles)
	roleService.authorization = authorization
	roleService.transactionActive = runtime.AuthTransactionActive
	if err := configureRoleTransaction(roleService, runtime, config.Database.DB().Pool()); err != nil {
		if authorization != nil {
			authorization.Close()
		}
		_ = db.Close()
		return nil, fmt.Errorf("goadmin auth: create role transaction: %w", err)
	}

	adapter := &Adapter{
		authorization: authorization,
		runtime:       runtime, roles: roleService, memberships: config.Memberships, db: db,
		stateKeys: config.Runtime.OutboxAEADKeys, database: config.Database,
	}
	if authorization != nil {
		if target, ok := config.Memberships.(membershipMutationConfigurer); ok {
			adapter.configureMembershipInvalidation(target)
		}
	}
	return adapter, nil
}

func adminMembershipGate(upstream goauth.MembershipGate, memberships AdminMemberships) goauth.MembershipGate {
	return goauth.MembershipGateFunc(
		func(ctx context.Context, realm goauth.Realm, account goauth.Account) error {
			if realm == goauth.RealmUser {
				return nil
			}
			if upstream != nil {
				if err := upstream.AllowMembership(ctx, realm, account); err != nil {
					return err
				}
			} else if realm != goauth.RealmAdmin {
				return goauth.ErrMembershipDenied
			}
			if realm != goauth.RealmAdmin {
				return nil
			}
			allowed, err := memberships.HasAdminMembership(ctx, account.Subject.ID)
			if err != nil {
				return fmt.Errorf("check admin membership: %w", err)
			}
			if !allowed {
				return goauth.ErrMembershipDenied
			}

			return nil
		},
	)
}

func (a *Adapter) BrowserState(backend string, conn redis.UniversalClient) (browserstate.Store, error) {
	switch backend {
	case "", "redis":
		if conn == nil {
			return nil, errors.New("admin Redis browser state requires SessionRedis")
		}
		return browserstate.NewRedis(conn, a.stateKeys), nil
	case "pgsql":
		return browserstate.NewPostgres(a.db, a.stateKeys), nil
	default:
		return nil, errors.New("unsupported admin browser state backend")
	}
}

// ProvisionTrustedAdmin idempotently ensures a verified local account and its
// host-owned admin membership. Existing accounts must prove the same password;
// a seed never links by email alone.
func (a *Adapter) ProvisionTrustedAdmin(
	ctx context.Context,
	request goauth.RegisterRequest,
	publicID identity.UserID,
) (goauth.Account, int64, error) {
	if a == nil || a.runtime == nil || a.memberships == nil {
		return goauth.Account{}, 0, errors.New("goadmin auth adapter is not initialized")
	}
	prepared, err := PrepareTrustedAccount(ctx, a.runtime, request)
	if err != nil {
		return goauth.Account{}, 0, err
	}
	var account goauth.Account
	var membershipID int64
	err = a.runtime.InAuthTransaction(ctx, func(txCtx context.Context) error {
		executor, err := a.runtime.SQLExecutor(txCtx)
		if err != nil {
			return err
		}
		txCtx = admintx.WithExecutor(txCtx, executor, a.database.DB().Pool())
		account, err = prepared(txCtx)
		if err != nil {
			return err
		}
		membershipID, err = a.memberships.ProvisionAdminMembership(txCtx, account, publicID)
		return err
	})
	if err != nil {
		return goauth.Account{}, 0, err
	}
	return account, membershipID, nil
}

func (a *Adapter) Runtime() *postgres.Runtime {
	if a == nil {
		return nil
	}

	return a.runtime
}

func (a *Adapter) Roles() *RoleService {
	if a == nil {
		return nil
	}

	return a.roles
}

func (a *Adapter) Close() error {
	if a == nil || a.db == nil {
		return nil
	}

	if a.authorization != nil {
		a.authorization.Close()
	}
	return a.db.Close()
}
