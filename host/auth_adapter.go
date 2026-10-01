package host

import (
	"errors"
	"fmt"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/postgres"

	adminrepo "github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminrepo"
	internalauth "github.com/assurrussa/goadmin/internal/auth"
)

// AuthAdapterConfig is the single host-facing DI contract for goauth Runtime,
// admin memberships, and RBAC.
type AuthAdapterConfig struct {
	Database           StoragePgsqlClient
	TxManager          StoragePgsqlTxManager
	Runtime            goauth.Config
	AutoMigrate        bool
	NotificationSender goauth.NotificationSender
	NotificationWorker postgres.NotificationWorkerConfig
}

// AuthAdapter owns goadmin's supported integration with goauth v0.2. Hosts do
// not construct concrete auth repositories or import goadmin implementation
// packages.
type AuthAdapter struct {
	inner           *internalauth.Adapter
	admins          *adminrepo.Repo
	database        StoragePgsqlClient
	deliveryEnabled bool
}

func NewAuthAdapter(config AuthAdapterConfig) (*AuthAdapter, error) {
	if config.Database == nil {
		return nil, errors.New("goadmin auth adapter database is required")
	}
	if config.TxManager == nil {
		return nil, errors.New("goadmin auth adapter transaction manager is required")
	}
	admins, err := adminrepo.New(adminrepo.NewOptions(config.Database, config.TxManager))
	if err != nil {
		return nil, fmt.Errorf("goadmin auth adapter: build admin memberships: %w", err)
	}
	inner, err := internalauth.New(internalauth.Config{
		Database: config.Database, Runtime: config.Runtime,
		Memberships: admins, AutoMigrate: config.AutoMigrate,
		NotificationSender: config.NotificationSender, NotificationWorker: config.NotificationWorker,
	})
	if err != nil {
		return nil, err
	}

	return &AuthAdapter{
		inner: inner, admins: admins, database: config.Database,
		deliveryEnabled: config.Runtime.NotificationDelivery != goauth.NotificationDeliveryDisabled && config.NotificationSender != nil,
	}, nil
}

// Runtime exposes the supported PostgreSQL goauth adapter for custom host
// wiring such as OIDC and user-realm endpoints.
func (a *AuthAdapter) Runtime() *postgres.Runtime {
	if a == nil || a.inner == nil {
		return nil
	}

	return a.inner.Runtime()
}

func (a *AuthAdapter) Close() error {
	if a == nil || a.inner == nil {
		return nil
	}

	return a.inner.Close()
}

func (a *AuthAdapter) valid() bool {
	return a != nil && a.inner != nil && a.admins != nil && a.Runtime() != nil
}
