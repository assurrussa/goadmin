package operations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/host"
)

// DefaultFrontendScope is the default state scope used by the operations page.
const DefaultFrontendScope = "frontend"

var (
	// ErrFrontendRebuildDeferredByDebounce reports that a rebuild was blocked by debounce.
	ErrFrontendRebuildDeferredByDebounce = errors.New("frontend rebuild deferred by debounce")
	// ErrFrontendRebuildDeferredByLock reports that a rebuild was blocked by an active lock.
	ErrFrontendRebuildDeferredByLock = errors.New("frontend rebuild deferred by active lock")
)

// RoutesRegenerator runs a one-shot route regeneration pass.
type RoutesRegenerator interface {
	Regenerate(ctx context.Context) error
}

// SitemapRegenerator runs a one-shot sitemap regeneration pass.
type SitemapRegenerator interface {
	Regenerate(ctx context.Context) error
}

// FrontendBuildState is the host-facing frontend rebuild state contract.
type FrontendBuildState struct {
	Scope                string
	Dirty                bool
	Revision             int64
	LastMutationAt       *time.Time
	LastRebuildRequestAt *time.Time
	BuildLockUntil       *time.Time
	UpdatedAt            time.Time
}

// FrontendStateReader loads the current frontend build state.
type FrontendStateReader interface {
	Get(ctx context.Context, scope string) (FrontendBuildState, error)
}

// FrontendRedeployer requests a frontend rebuild/redeploy.
type FrontendRedeployer interface {
	Enabled() bool
	HasTriggerConfig() bool
	TriggerNow(ctx context.Context, reason string) error
}

// Handler mounts the reusable operations admin routes.
type Handler struct {
	adminApp        *host.App
	routePath       string
	menuExtra       host.Menu
	routes          RoutesRegenerator
	sitemap         SitemapRegenerator
	frontendState   FrontendStateReader
	frontendRebuild FrontendRedeployer
}

// NewHandler creates a reusable operations handler set.
func NewHandler(
	adminApp *host.App,
	routes RoutesRegenerator,
	sitemap SitemapRegenerator,
	frontendState FrontendStateReader,
	frontendRebuild FrontendRedeployer,
	menuExtra host.Menu,
) *Handler {
	return &Handler{
		adminApp:        adminApp,
		routePath:       "/operations",
		menuExtra:       menuExtra,
		routes:          routes,
		sitemap:         sitemap,
		frontendState:   frontendState,
		frontendRebuild: frontendRebuild,
	}
}

// RegisterGroupRoutes mounts the operations routes.
func (h *Handler) RegisterGroupRoutes(route fiber.Router, _ ...fiber.Handler) {
	group := route.Group(h.routePath)
	readGuard := h.adminApp.Guard(host.PermissionDomainOperations, host.PermissionActionRead)
	updateGuard := h.adminApp.Guard(host.PermissionDomainOperations, host.PermissionActionUpdate)

	group.Get("", readGuard, h.Index)
	group.Post("routes/regenerate", updateGuard, h.RegenerateRoutes)
	group.Post("sitemap/regenerate", updateGuard, h.RegenerateSitemap)
	group.Post("frontend/redeploy", updateGuard, h.RedeployFrontend)
}

// Index renders the operations dashboard.
func (h *Handler) Index(c fiber.Ctx) error {
	state, err := h.frontendState.Get(c, DefaultFrontendScope)
	if err != nil {
		return fmt.Errorf("load frontend build state: %w", err)
	}

	host.WithBreadcrumbs(c, h.adminApp, h.menuExtra, []host.Breadcrumb{
		{Name: "Операции"}, //nolint:goconst // required
	})

	return h.adminApp.Render(c, "operations/IndexPage", map[string]any{
		"title": "Операции",
		"frontendState": map[string]any{
			"exists":               state.Scope != "",
			"scope":                state.Scope,
			"dirty":                state.Dirty,
			"revision":             state.Revision,
			"lastMutationAt":       formatTime(state.LastMutationAt),
			"lastRebuildRequestAt": formatTime(state.LastRebuildRequestAt),
			"buildLockUntil":       formatTime(state.BuildLockUntil),
			"updatedAt":            formatZeroTime(state.UpdatedAt),
		},
		"frontendRebuild": map[string]any{
			"enabled":          h.frontendRebuild.Enabled(),
			"hasTriggerConfig": h.frontendRebuild.HasTriggerConfig(),
			"canTrigger":       h.frontendRebuild.Enabled() && h.frontendRebuild.HasTriggerConfig(),
		},
	})
}

// RegenerateRoutes triggers a manual route regeneration.
func (h *Handler) RegenerateRoutes(c fiber.Ctx) error {
	ctx, cancel := newDetachedOperationContext()
	defer cancel()

	if err := h.routes.Regenerate(ctx); err != nil {
		h.adminApp.WithFlashError(c, "Не удалось пересобрать routes-front")
		return h.adminApp.RedirectBack(c)
	}

	h.adminApp.WithFlashSuccess(c, "Публичные routes-front пересобраны")
	return h.adminApp.RedirectBack(c)
}

// RegenerateSitemap triggers a manual sitemap regeneration.
func (h *Handler) RegenerateSitemap(c fiber.Ctx) error {
	ctx, cancel := newDetachedOperationContext()
	defer cancel()

	if err := h.sitemap.Regenerate(ctx); err != nil {
		h.adminApp.WithFlashError(c, "Не удалось пересобрать sitemap")
		return h.adminApp.RedirectBack(c)
	}

	h.adminApp.WithFlashSuccess(c, "Sitemap пересобран")
	return h.adminApp.RedirectBack(c)
}

// RedeployFrontend triggers a manual frontend rebuild request.
func (h *Handler) RedeployFrontend(c fiber.Ctx) error {
	if !h.frontendRebuild.Enabled() {
		h.adminApp.WithFlashWarning(c, "Frontend rebuild выключен в конфиге")
		return h.adminApp.RedirectBack(c)
	}
	if !h.frontendRebuild.HasTriggerConfig() {
		h.adminApp.WithFlashWarning(c, "Frontend rebuild trigger не настроен")
		return h.adminApp.RedirectBack(c)
	}

	ctx, cancel := newDetachedOperationContext()
	defer cancel()

	if err := h.frontendRebuild.TriggerNow(ctx, "Manual admin-triggered rebuild"); err != nil {
		switch {
		case errors.Is(err, ErrFrontendRebuildDeferredByDebounce):
			h.adminApp.WithFlashWarning(c, "Frontend rebuild отложен debounce-окном после недавнего изменения контента")
			return h.adminApp.RedirectBack(c)
		case errors.Is(err, ErrFrontendRebuildDeferredByLock):
			h.adminApp.WithFlashWarning(c, "Frontend rebuild уже запущен и пока находится под lock")
			return h.adminApp.RedirectBack(c)
		}

		h.adminApp.WithFlashError(c, "Не удалось запросить redeploy фронтенда")
		return h.adminApp.RedirectBack(c)
	}

	h.adminApp.WithFlashSuccess(c, "Запрос на redeploy фронтенда отправлен")
	return h.adminApp.RedirectBack(c)
}

func formatTime(value *time.Time) string {
	if value == nil {
		return ""
	}

	return value.UTC().Format(time.RFC3339)
}

func formatZeroTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}

	return value.UTC().Format(time.RFC3339)
}
