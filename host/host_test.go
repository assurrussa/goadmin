package host_test

import (
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"

	adminhost "github.com/assurrussa/goadmin/host"
)

type stubRegistry struct {
	menu                  adminhost.Menu
	permissionDefinitions []adminhost.PermissionDefinition
	builders              []adminhost.ExtensionBuilder
}

const mutatedValue = "mutated"

func (s stubRegistry) Menu() adminhost.Menu {
	return s.menu
}

func (s stubRegistry) PermissionDefinitions() []adminhost.PermissionDefinition {
	return s.permissionDefinitions
}

func (s stubRegistry) ExtensionBuilders(adminhost.Menu) []adminhost.ExtensionBuilder {
	return s.builders
}

const testProductionEnvironment = "production"

func TestBuildConfig_UsesHostDefaults(t *testing.T) {
	t.Parallel()

	cfg := adminhost.BuildConfig(adminhost.AdminConfigInput{
		Env:                testProductionEnvironment,
		BaseDomain:         "example.com",
		BaseDomainURL:      "https://example.com", //nolint:goconst // required
		Domain:             "admin.example.com",
		DomainURL:          "https://admin.example.com",
		Addr:               ":8078",
		CSRFTokenName:      "csrf_token",
		CSRFTokenTTL:       time.Hour,
		SessionInactiveTTL: 30 * time.Minute,
		SessionTTL:         24 * time.Hour,
		SessionName:        "sess_id",
	}, adminhost.PathDefaults{
		PublicRoot:     "internal/admin/publicstaticdata",
		StaticDataRoot: "publicdata",
		ViewsRoot:      "internal/admin/views",
	})

	require.Equal(t, "pgsql", cfg.CanonicalSessionBackend)
	require.Equal(t, "internal/admin/publicstaticdata", cfg.PublicRoot)
	require.Equal(t, "internal/admin/publicstaticdata/hot", cfg.HotFile)
	require.Equal(t, "internal/admin/views", cfg.ViewsRoot)
	require.Equal(t, "publicdata", cfg.StaticDataRoot)
}

func TestBuildConfig_PreservesExplicitPathsAndClonesSlices(t *testing.T) {
	t.Parallel()

	basicAuth := []string{"admin:secret"}
	cfg := adminhost.BuildConfig(adminhost.AdminConfigInput{
		BasicAuth:      basicAuth,
		PublicRoot:     "custom/public",
		StaticDataRoot: "custom/publicdata",
		HotFile:        "custom/hot",
		ViewsRoot:      "custom/views",
	}, adminhost.PathDefaults{})
	basicAuth[0] = mutatedValue

	require.Equal(t, []string{"admin:secret"}, cfg.BasicAuth)
	require.Equal(t, "custom/public", cfg.PublicRoot)
	require.Equal(t, "custom/publicdata", cfg.StaticDataRoot)
	require.Equal(t, "custom/hot", cfg.HotFile)
	require.Equal(t, "custom/views", cfg.ViewsRoot)
}

func TestBuildBootstrapConfig_FallsBackToAdminAddr(t *testing.T) {
	t.Parallel()

	cfg := adminhost.BuildBootstrapConfig(adminhost.BootstrapConfigInput{
		Admin: adminhost.AdminConfigInput{
			Env:    "staging",
			Addr:   ":8078",
			Domain: "admin.example.com",
		},
		Server: adminhost.ServerConfigInput{
			AllowOrigins: []string{"https://example.com"},
			ReadTimeout:  5 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: time.Minute,
		},
	})

	require.Equal(t, "staging", cfg.Env)
	require.Equal(t, ":8078", cfg.AdminConfig.Addr)
	require.Equal(t, "publicstaticdata", cfg.AdminConfig.PublicRoot)
	require.Equal(t, "publicdata", cfg.AdminConfig.StaticDataRoot)
	require.Equal(t, "publicstaticdata/hot", cfg.AdminConfig.HotFile)
	require.Equal(t, ":8078", cfg.ServerConfig.Addr)
	require.Equal(t, []string{"https://example.com"}, cfg.ServerConfig.AllowOrigins)
	require.Equal(t, 5*time.Second, cfg.ServerConfig.ReadTimeout)
	require.Equal(t, 15*time.Second, cfg.ServerConfig.WriteTimeout)
	require.Equal(t, time.Minute, cfg.ServerConfig.IdleTimeout)
}

func TestBuildBootstrapOptionsPlan_CollectsMenuBuildersAndPublicFSRules(t *testing.T) {
	t.Parallel()

	templateFS := fstest.MapFS{
		"app.gohtml": {Data: []byte("ok")},
	}
	publicFS := fstest.MapFS{
		"dist/app.js": {Data: []byte("ok")},
	}
	builder := func(*adminhost.App) (*adminhost.Extension, error) {
		return adminhost.NewExtension(), nil
	}

	productionPlan := adminhost.BuildBootstrapOptionsPlan(adminhost.BootstrapOptionsInput{
		Env:        "production",
		TemplateFS: templateFS,
		PublicFS:   publicFS,
		Registry: stubRegistry{
			menu: adminhost.Menu{
				Sections: []adminhost.Section{{Key: "system"}}, //nolint:goconst // required
			},
			permissionDefinitions: []adminhost.PermissionDefinition{
				adminhost.NewPermissionDefinition(
					adminhost.NewPermissionKey(adminhost.PermissionDomainOperations, adminhost.PermissionActionUpdate),
					"Run operations",
				),
			},
			builders: []adminhost.ExtensionBuilder{builder},
		},
	})

	require.Equal(t, adminhost.DefaultAuthSleep, productionPlan.AuthSleep)
	require.True(t, productionPlan.IncludePublicFS)
	require.Equal(t, "system", productionPlan.Menu.Sections[0].Key)
	require.Len(t, productionPlan.PermissionDefinitions, 1)
	require.Equal(t, "Run operations", productionPlan.PermissionDefinitions[0].Description)
	require.Len(t, productionPlan.ExtensionBuilders, 1)

	templateBody, err := fs.ReadFile(productionPlan.TemplateFS, "app.gohtml")
	require.NoError(t, err)
	require.Equal(t, "ok", string(templateBody))

	publicBody, err := fs.ReadFile(productionPlan.PublicFS, "dist/app.js")
	require.NoError(t, err)
	require.Equal(t, "ok", string(publicBody))

	localPlan := adminhost.BuildBootstrapOptionsPlan(adminhost.BootstrapOptionsInput{
		Env:        "local",
		TemplateFS: templateFS,
		PublicFS:   publicFS,
	})

	require.False(t, localPlan.IncludePublicFS)
	require.Len(t, localPlan.Options(), 3)
	require.False(t, adminhost.IsLocalEnv("production"))
	require.True(t, adminhost.IsLocalEnv("development"))
}

func TestEnvNameAccessors_ReturnClonedContracts(t *testing.T) {
	t.Parallel()

	runtimeNames := adminhost.RuntimeEnvNames()
	buildNames := adminhost.BuildEnvNames()

	require.Equal(t, []string{
		adminhost.EnvAppAdminDomain,
		adminhost.EnvAppAdminDomainURL,
		adminhost.EnvAppAdminCSRFTokenName,
		adminhost.EnvAppAdminCSRFTokenTTL,
		adminhost.EnvAppAdminSessionInactiveTTL,
		adminhost.EnvAppAdminSessionTTL,
		adminhost.EnvAppAdminSessionName,
		adminhost.EnvAppAdminCanonicalSessionBackend,
		adminhost.EnvServerAdminAddr,
		adminhost.EnvServerAdminBasicAuth,
		adminhost.EnvGoAdminPublicRoot,
		adminhost.EnvGoAdminHotFile,
		adminhost.EnvGoAdminViewsRoot,
	}, runtimeNames)
	require.Equal(t, []string{
		adminhost.EnvViteAdminExtRoot,
		adminhost.EnvViteAdminExtensionManifests,
		adminhost.EnvViteAdminCSRFCookieName,
		adminhost.EnvViteAdminPublicRoot,
		adminhost.EnvViteAdminTSConfigPath,
	}, buildNames)

	runtimeNames[0] = "mutated"
	buildNames[0] = "mutated"

	require.Equal(t, adminhost.EnvAppAdminDomain, adminhost.RuntimeEnvNames()[0])
	require.Equal(t, adminhost.EnvViteAdminExtRoot, adminhost.BuildEnvNames()[0])
}

func TestInstallDependencyTypeAliasesCompile(t *testing.T) {
	t.Parallel()

	var _ adminhost.Logger
	var _ *adminhost.Server
	var _ adminhost.CSRFRequest
	var _ adminhost.CSRFToken
	var _ adminhost.CSRFClaims
	var _ *adminhost.CSRFService
	var _ adminhost.EventStream
	var _ *adminhost.SessionStore
	var _ adminhost.RedisClient
	var _ adminhost.NotificationManager
	var _ adminhost.StoragePgsqlClient
	var _ adminhost.StoragePgsqlTxManager
	var _ adminhost.AfterProcessFunc
	var _ adminhost.AfterProcessPayload
	var _ adminhost.BatchRequest
	var _ adminhost.DeleteRequest
	var _ adminhost.File
	var _ adminhost.FileData
	var _ adminhost.FileEventAfterJob
	var _ adminhost.FileUploadConfig
	var _ adminhost.ListFilters
	var _ adminhost.ObjectID
	var _ adminhost.ObjectType
	var _ adminhost.ReaderRequest
	var _ adminhost.ReaderUploadInput
	var _ adminhost.SingleRequest
	var _ adminhost.TaskUploader
	var _ adminhost.TusStore
	var _ adminhost.AfterProcessHandler
	var _ *adminhost.AfterProcessService
	var _ adminhost.UploadedFile
	var _ adminhost.UserID
}

func TestBuildBootstrapConfigPreservesHTTPTimeouts(t *testing.T) {
	t.Parallel()
	cfg := adminhost.BuildBootstrapConfig(adminhost.BootstrapConfigInput{
		Server: adminhost.ServerConfigInput{ReadTimeout: time.Second, WriteTimeout: 2 * time.Second, IdleTimeout: 3 * time.Second},
	})
	require.Equal(t, time.Second, cfg.ServerConfig.ReadTimeout)
	require.Equal(t, 2*time.Second, cfg.ServerConfig.WriteTimeout)
	require.Equal(t, 3*time.Second, cfg.ServerConfig.IdleTimeout)
}
