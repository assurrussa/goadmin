package dashboard

import (
	"context"
	"log/slog"

	logger "github.com/assurrussa/gologger"
	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/adminapp"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	cards "github.com/assurrussa/goadmin/internal/dashboard"
	"github.com/assurrussa/goadmin/shared"
)

const titleProp = "title"

// Handler обрабатывает запросы, связанные с аутентификацией.
type Handler struct {
	adminApp  *adminapp.App
	dashboard *cards.Registry
}

// NewHandler создает новый обработчик аутентификации.
func NewHandler(adminApp *adminapp.App, dashboards ...*cards.Registry) *Handler {
	var registry *cards.Registry
	if len(dashboards) > 0 {
		registry = dashboards[0]
	}
	return &Handler{
		adminApp:  adminApp,
		dashboard: registry,
	}
}

func (h *Handler) RegisterGroupRoutes(router fiber.Router, _ ...fiber.Handler) {
	router.Get("/dashboard/data",
		h.adminApp.Guard(integrationroles.PermissionDomainDashboard, integrationroles.PermissionActionRead), h.Data)
	router.Get("/", h.adminApp.Guard(integrationroles.PermissionDomainDashboard, integrationroles.PermissionActionRead), h.Dashboard)
	if h.adminApp.Env() == "local" || h.adminApp.Env() == "development" || h.adminApp.Env() == "dev" {
		router.Get("/test/theme", h.adminApp.Guard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionRead), h.TestTheme)     //nolint:lll // required
		router.Get("/test/buttons", h.adminApp.Guard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionRead), h.TestButtons) //nolint:lll // required
	}
}

func (h *Handler) Dashboard(c fiber.Ctx) error {
	return h.adminApp.HTTPManager().Render(c, "dashboard/IndexPage", map[string]any{
		titleProp:   "Главная",
		"dashboard": h.authorized(c).Descriptors(),
	})
}

// Data reauthorizes all descriptors and links before starting any providers.
func (h *Handler) Data(c fiber.Ctx) error {
	c.Set(fiber.HeaderCacheControl, "no-store")
	admin := shared.GetAdminAuth(c)
	if admin == nil || admin.ID <= 0 || admin.AuthSubjectID().IsZero() {
		return fiber.ErrUnauthorized
	}
	request := h.authorized(c)
	adminID := admin.ID
	log := h.adminApp.Logger()
	snapshot := request.Load(c.Context(), func(ctx context.Context, key string, err error) {
		log.ErrorContext(ctx, "load dashboard card", slog.String("card", key),
			slog.Int64("admin_id", adminID), logger.Error(err))
	})
	return c.JSON(snapshot)
}

func (h *Handler) authorized(c fiber.Ctx) cards.Request {
	admin := shared.GetAdminAuth(c)
	var actor cards.Actor
	if admin != nil && !admin.AuthSubjectID().IsZero() {
		actor = cards.Actor{AdminID: admin.ID, SubjectID: admin.AuthSubjectID().String()}
	}
	return h.dashboard.Authorize(actor, func(key integrationroles.PermissionKey) bool {
		return integrationroles.RolePreviewAllows(c, key) && h.adminApp.RolesService().AdminGuardCheck(c, actor.AdminID, key)
	})
}

func (h *Handler) TestTheme(c fiber.Ctx) error {
	return h.adminApp.HTTPManager().Render(c, "test/ThemePage", map[string]any{
		titleProp: "Тест темы в админке",
	})
}

func (h *Handler) TestButtons(c fiber.Ctx) error {
	return h.adminApp.HTTPManager().Render(c, "test/ButtonsPage", map[string]any{
		titleProp: "Тест конпок в админке",
	})
}
