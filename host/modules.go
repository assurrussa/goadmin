package host

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/assurrussa/goauth"
	"github.com/assurrussa/goauth/postgres"

	"github.com/assurrussa/goadmin/bootstrap"
)

const (
	jobsModuleKey     = "jobs"
	authMailModuleKey = "authmail"
	coreModuleKey     = "core"
)

// ModuleDescriptor declares dependencies and HTTP ownership relative to the admin mount.
type ModuleDescriptor struct {
	Key      string
	Requires []string
	// Routes includes a method and a Fiber path, for example "GET /reports/:id".
	// Parameter and wildcard overlaps are rejected conservatively.
	Routes []string
	// RouteNamespaces reserves a literal path and all descendants, for every method.
	RouteNamespaces []string
}

// Module is an immutable assembly contribution. Product dependencies belong in
// a Feature constructor; only the narrow App facade is passed to its builder.
type Module struct {
	descriptor ModuleDescriptor
	configure  func(*assembly) error
	feature    Feature
}

func (m Module) Descriptor() ModuleDescriptor {
	d := m.descriptor
	d.Requires = slices.Clone(d.Requires)
	d.Routes = slices.Clone(d.Routes)
	d.RouteNamespaces = slices.Clone(d.RouteNamespaces)
	return d
}

// FeatureModule mounts a host feature and declares its module dependencies.
func FeatureModule(feature Feature, requires ...string) Module {
	if feature == nil {
		return Module{}
	}
	return Module{descriptor: ModuleDescriptor{Key: feature.Descriptor().Key, Requires: slices.Clone(requires)}, feature: feature}
}

// DefinedFeatureModule also declares route ownership for preflight conflict checks.
func DefinedFeatureModule(d ModuleDescriptor, feature Feature) Module {
	d.Requires = slices.Clone(d.Requires)
	d.Routes = slices.Clone(d.Routes)
	d.RouteNamespaces = slices.Clone(d.RouteNamespaces)
	return Module{descriptor: d, feature: feature}
}

func AccessModule() Module {
	return Module{descriptor: ModuleDescriptor{Key: "access", RouteNamespaces: []string{"/admins", "/roles", "/permissions"}}}
}

// JobsConfig either borrows an already supervised host queue or creates an
// admin-owned queue. Borrowed workers are never started, drained or closed here.
type JobsConfig struct {
	Borrowed Outbox
	Owned    OutboxRuntime
	Worker   OutboxConfig
}

func JobsModule(cfg JobsConfig) Module {
	return Module{descriptor: ModuleDescriptor{Key: jobsModuleKey}, configure: func(a *assembly) error {
		if cfg.Borrowed != nil && cfg.Owned != nil {
			return errors.New("jobs cannot be both owned and borrowed")
		}
		if cfg.Borrowed != nil {
			a.input.Services.Outbox = cfg.Borrowed
			return nil
		}
		if cfg.Owned != nil {
			a.input.Services.Outbox = cfg.Owned
			a.workers = append(a.workers, runtimeWorker{
				name: jobsModuleKey, run: cfg.Owned.Run, ready: cfg.Owned.Readiness, drain: cfg.Owned.BeginDrain,
			})
			return nil
		}
		worker, err := NewOutboxRuntime(a.input.Services.TxManager, a.input.Services.DB, a.input.Services.Logger, cfg.Worker)
		if err != nil {
			return err
		}
		a.input.Services.Outbox = worker
		a.workers = append(a.workers, runtimeWorker{
			name: jobsModuleKey, run: worker.Run, ready: worker.Readiness, drain: worker.BeginDrain,
		})
		return nil
	}}
}

type UploadsConfig struct {
	Repository FileRepository
	Loader     FileLoader
	Uploads    Uploads
	URLs       URLConfigs
	Jobs       []Job
}

func UploadsModule(cfg UploadsConfig) Module {
	return Module{descriptor: ModuleDescriptor{
		Key: "uploads", Requires: []string{jobsModuleKey},
		RouteNamespaces: []string{"/files", "/uploads"}, Routes: []string{"DELETE /auth/profile/avatar"},
	}, configure: func(a *assembly) error {
		if cfg.Repository == nil || cfg.Loader == nil || cfg.Uploads.Service == nil ||
			cfg.Uploads.TusStore == nil || cfg.Uploads.AfterProcess == nil {
			return errors.New("uploads requires repository, loader, uploader, TUS store and after-process service")
		}
		if _, ok := cfg.Repository.(interface {
			GetByPath(ctx context.Context, path string) (File, error)
		}); !ok {
			return errors.New("uploads repository requires indexed GetByPath")
		}
		if _, ok := cfg.Repository.(interface {
			BindPreview(ctx context.Context, binding PreviewBinding) error
		}); !ok {
			return errors.New("uploads repository requires atomic BindPreview")
		}
		a.input.Repositories.FileRepo = cfg.Repository
		a.input.Repositories.FileLoader = cfg.Loader
		a.input.Uploads = cfg.Uploads
		a.input.URLs = cfg.URLs
		a.input.Options = append(a.input.Options, bootstrap.WithOutboxJobs(cfg.Jobs...))
		return nil
	}}
}

func QueuesModule() Module {
	return Module{descriptor: ModuleDescriptor{
		Key: "queues", Requires: []string{jobsModuleKey}, RouteNamespaces: []string{"/queues"},
	}}
}

func NotificationsModule(manager NotificationManager) Module {
	return Module{descriptor: ModuleDescriptor{
		Key: "notifications", Requires: []string{jobsModuleKey}, RouteNamespaces: []string{"/notifications"},
	}, configure: func(a *assembly) error {
		if manager == nil {
			return errors.New("notifications requires a durable delivery transport")
		}
		a.input.Services.Notifier = manager
		return nil
	}}
}

// RealtimeModule borrows a supplied stream. A nil stream creates one owned by
// each runtime, which shuts it down after its WebSocket handler.
func RealtimeModule(stream EventStream) Module {
	return Module{descriptor: ModuleDescriptor{Key: "realtime", Routes: []string{"GET /ws"}}, configure: func(a *assembly) error {
		configured := stream
		if stream == nil {
			owned := NewEventStream()
			configured = owned
			a.shutdownEventStream = owned.Shutdown
		}
		a.input.Services.EventStream = configured
		return nil
	}}
}

type AuthMailConfig struct {
	Sender goauth.NotificationSender
	Worker postgres.NotificationWorkerConfig
	// BorrowedWorker means the host already supervises this Runtime's delivery.
	BorrowedWorker bool
}

func AuthMailModule(cfg AuthMailConfig) Module {
	return Module{descriptor: ModuleDescriptor{
		Key: authMailModuleKey, Routes: []string{
			"GET /auth/forgot-password", "POST /auth/forgot-password",
			"GET /reset-password", "POST /reset-password", "GET /auth/reset-password", "POST /auth/reset-password",
			"POST /auth/profile/settings/email/request", "POST /auth/profile/settings/email/confirm",
		},
	}, configure: func(a *assembly) error {
		if cfg.BorrowedWorker && a.borrowedAuth == nil {
			return errors.New("borrowed authmail worker requires borrowed Auth")
		}
		if a.borrowedAuth != nil {
			if !a.borrowedAuth.deliveryEnabled {
				return errors.New("borrowed Auth does not support managed mail delivery")
			}
			if cfg.Sender != nil || !reflect.ValueOf(cfg.Worker).IsZero() {
				return errors.New("borrowed Auth owns its sender and worker configuration")
			}
		}
		if cfg.Sender == nil && a.borrowedAuth == nil {
			return errors.New("authmail requires a sender")
		}
		a.mail = &cfg
		return nil
	}}
}

func validateModules(modules []Module) (map[string]bool, []Module, error) {
	claims, err := moduleRouteClaims(ModuleDescriptor{
		Key: coreModuleKey,
		Routes: []string{
			"GET /", "GET /auth/login", "POST /auth/login", "GET /auth/register", "POST /auth/register", "GET /auth/profile",
			"DELETE /auth/logout", "GET /auth/profile/edit", "POST /auth/profile/edit",
			"GET /auth/profile/settings", "POST /auth/profile/settings/password",
		},
		// Reserve development pages in every environment to keep ownership stable.
		RouteNamespaces: []string{"/public", "/test"},
	})
	if err != nil {
		return nil, nil, err
	}
	byKey, err := indexModules(modules, claims)
	if err != nil {
		return nil, nil, err
	}
	state := map[string]int{}
	capabilities := map[string]bool{}
	ordered := make([]Module, 0, len(modules))
	var visit func(string) error
	visit = func(key string) error {
		if state[key] == 2 {
			return nil
		}
		if state[key] == 1 {
			return fmt.Errorf("module dependency cycle at %q", key)
		}
		m, ok := byKey[key]
		if !ok {
			return fmt.Errorf("missing module dependency %q", key)
		}
		state[key] = 1
		for _, dep := range m.descriptor.Requires {
			if err := visit(dep); err != nil {
				return fmt.Errorf("module %q: %w", key, err)
			}
		}
		state[key] = 2
		capabilities[key] = true
		ordered = append(ordered, m)
		return nil
	}
	for _, m := range modules {
		if err := visit(m.descriptor.Key); err != nil {
			return nil, nil, err
		}
	}
	return capabilities, ordered, nil
}

func indexModules(modules []Module, claims []routeClaim) (map[string]Module, error) {
	byKey := make(map[string]Module, len(modules))
	for _, m := range modules {
		d := m.descriptor
		if m.feature != nil {
			switch d.Key {
			case "core", "access", "jobs", "uploads", "queues", "notifications", "realtime", "authmail":
				return nil, fmt.Errorf("reserved built-in module key %q", d.Key)
			}
		}
		if strings.TrimSpace(d.Key) == "" || strings.TrimSpace(d.Key) != d.Key || d.Key == "core" {
			return nil, fmt.Errorf("invalid module key %q", d.Key)
		}
		if _, ok := byKey[d.Key]; ok {
			return nil, fmt.Errorf("duplicate module %q", d.Key)
		}
		byKey[d.Key] = m
		if m.feature != nil && m.feature.Descriptor().Key != d.Key {
			return nil, fmt.Errorf("module %q does not match feature key", d.Key)
		}
		owned, err := moduleRouteClaims(d)
		if err != nil {
			return nil, err
		}
		for _, claim := range owned {
			for _, previous := range claims {
				if routeClaimsOverlap(previous, claim) {
					return nil, fmt.Errorf("route %q overlaps %q: claimed by %s and %s", claim.label, previous.label, previous.owner, d.Key)
				}
			}
		}
		claims = append(claims, owned...)
	}
	return byKey, nil
}
