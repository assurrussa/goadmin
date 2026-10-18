package profile

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"sync/atomic"

	"github.com/assurrussa/goauth/postgres"
	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/host"
	authcore "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/presentation"
	"github.com/assurrussa/goadmin/models"
)

const (
	sourcePath    = "/profile-source"
	componentKey  = "profile-source"
	refreshSuffix = "/refresh"
	cancelSuffix  = "/cancel"
)

type Config struct {
	Runtime     *postgres.Runtime
	Issuer      string
	ClientID    string
	ProjectID   string
	AvatarHosts []string
	// Start navigates to the host's exact state-bound SDK landing using ONLY
	// the opaque selector. The host must request a new callback, never cached Verify.
	Start func(ctx fiber.Ctx, selector string) error
}

type Component struct {
	cfg     Config
	avatars avatarPolicy
	built   atomic.Bool
}

func New(ctx context.Context, cfg Config) (*Component, error) {
	u, err := url.Parse(cfg.Issuer)
	if ctx == nil || err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil ||
		u.Path != "" || u.RawQuery != "" || u.Fragment != "" || cfg.ClientID == "" || cfg.ProjectID == "" ||
		cfg.Runtime == nil || cfg.Runtime.Runtime == nil || cfg.Runtime.Database() == nil || cfg.Start == nil ||
		ValidateAvatarHosts(cfg.AvatarHosts) != nil {
		return nil, ErrInvalid
	}
	rows, err := cfg.Runtime.Database().QueryContext(ctx, "SELECT "+columns+" FROM admin_profile_presentations LIMIT 0")
	if err != nil {
		return nil, errors.New("profile presentation schema is not ready")
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	cfg.AvatarHosts = slices.Clone(cfg.AvatarHosts)
	return &Component{cfg: cfg, avatars: avatarPolicy{hosts: cfg.AvatarHosts}}, nil
}

func (c *Component) Module() host.Module {
	return host.DefinedFeatureModule(host.ModuleDescriptor{Key: componentKey, RouteNamespaces: []string{sourcePath}}, c)
}

func (c *Component) Descriptor() host.FeatureDescriptor {
	return host.FeatureDescriptor{Key: componentKey, Menu: host.Menu{Sections: []host.Section{{
		Key: componentKey, Title: "Profile", Placement: "account",
		Items: []host.Item{{Name: "Profile source", Href: sourcePath, Icon: "user"}},
	}}}}
}

func (c *Component) Build(host.Menu) host.ExtensionBuilder {
	return func(app *host.App) (*host.Extension, error) {
		if app == nil || !app.Unwrap().AdminAuth().UsesCanonicalRuntime(c.cfg.Runtime) {
			return nil, ErrInvalid
		}
		if !c.built.CompareAndSwap(false, true) {
			return nil, errors.New("profile component already mounted")
		}
		h := &handler{component: c, app: app}
		return host.NewExtension().WithHandlers(h).WithMiddlewares(func(ctx fiber.Ctx) error {
			presentation.Set(ctx, func() (View, error) {
				_, binding, err := c.current(ctx, app)
				if err != nil {
					return View{Enabled: true}, nil
				}
				return c.read(ctx, binding)
			})
			return ctx.Next()
		}), nil
	}
}

func (c *Component) current(ctx fiber.Ctx, app *host.App) (*models.SessionAdmin, host.ExternalBinding, error) {
	if app == nil || app.Unwrap() == nil || !app.Unwrap().AdminAuth().UsesCanonicalRuntime(c.cfg.Runtime) ||
		len(ctx.Request().Header.PeekAll("Authorization")) != 0 {
		return nil, host.ExternalBinding{}, ErrDenied
	}
	admin, err := app.Unwrap().AdminAuth().GetAdminAuth(ctx)
	if err != nil || admin == nil {
		return nil, host.ExternalBinding{}, ErrDenied
	}
	binding, err := app.ExternalAdminBinding(ctx)
	if err != nil || binding == nil || binding.LocalSubject != admin.AuthSubjectID() ||
		binding.Issuer != c.cfg.Issuer || binding.ClientID != c.cfg.ClientID || binding.ProjectID != c.cfg.ProjectID {
		return nil, host.ExternalBinding{}, ErrDenied
	}
	return admin, *binding, nil
}

type handler struct {
	component *Component
	app       *host.App
}

type selectionInput struct {
	Version string `json:"version"`
	Fields  Fields `json:"fields"`
}

func (h *handler) RegisterGroupRoutes(router fiber.Router, _ ...fiber.Handler) {
	router.Get(sourcePath, h.page)
	// The core mounted admin router validates native CSRF and Origin for writes.
	router.Post(sourcePath, h.selectSource)
	router.Post(sourcePath+refreshSuffix, h.refresh)
	router.Post(sourcePath+cancelSuffix, h.cancel)
}

func (h *handler) page(ctx fiber.Ctx) error {
	_, binding, err := h.component.current(ctx, h.app)
	if err != nil {
		return h.app.Render(ctx, "profile/SourcePage", map[string]any{
			"presentation": View{Enabled: true}, "avatarAvailable": len(h.component.avatars.hosts) > 0,
		})
	}
	view, err := h.component.read(ctx, binding)
	if err != nil {
		return fiber.ErrServiceUnavailable
	}
	return h.app.Render(ctx, "profile/SourcePage", map[string]any{
		"presentation": view, "avatarAvailable": len(h.component.avatars.hosts) > 0,
	})
}

func (h *handler) input(ctx fiber.Ctx) (selectionInput, host.ExternalBinding, error) {
	if authcore.RolePreviewActive(ctx) || len(ctx.Body()) > 2048 {
		return selectionInput{}, host.ExternalBinding{}, ErrDenied
	}
	_, binding, err := h.component.current(ctx, h.app)
	if err != nil {
		return selectionInput{}, host.ExternalBinding{}, err
	}
	var in selectionInput
	if err = ctx.Bind().Body(&in); err != nil {
		return selectionInput{}, host.ExternalBinding{}, err
	}
	return in, binding, nil
}

func (h *handler) selectSource(ctx fiber.Ctx) error {
	in, binding, err := h.input(ctx)
	if err != nil {
		return fiber.ErrForbidden
	}
	admin, _, err := h.component.current(ctx, h.app)
	if err != nil {
		return fiber.ErrForbidden
	}
	if err = h.component.selectFields(ctx, binding, in.Version, in.Fields, admin.AuthSessionID); err != nil {
		return presentationError(err)
	}
	return h.app.Redirect(ctx, sourcePath)
}

func (h *handler) refresh(ctx fiber.Ctx) error {
	in, _, err := h.input(ctx)
	if err != nil {
		return fiber.ErrForbidden
	}
	selector, err := h.component.CreateIntent(ctx, h.app, in.Version)
	if err != nil {
		return presentationError(err)
	}
	return h.component.cfg.Start(ctx, selector)
}

func (h *handler) cancel(ctx fiber.Ctx) error {
	if _, _, err := h.input(ctx); err != nil {
		return fiber.ErrForbidden
	}
	selector, err := Selector(ctx)
	if err != nil {
		return fiber.ErrForbidden
	}
	if err = h.component.Cancel(ctx, selector); err != nil {
		return fiber.ErrServiceUnavailable
	}
	clearRefreshCookie(ctx)
	return h.app.Redirect(ctx, sourcePath)
}

func presentationError(err error) error {
	if errors.Is(err, ErrConflict) {
		return fiber.ErrConflict
	}
	if errors.Is(err, ErrInvalid) {
		return fiber.ErrBadRequest
	}
	if errors.Is(err, ErrDenied) {
		return fiber.ErrForbidden
	}
	return fiber.ErrServiceUnavailable
}
