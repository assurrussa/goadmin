package dashboard

import (
	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/adminapp"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
)

const titleProp = "title"

// Handler обрабатывает запросы, связанные с аутентификацией.
type Handler struct {
	adminApp *adminapp.App
}

// NewHandler создает новый обработчик аутентификации.
func NewHandler(adminApp *adminapp.App) *Handler {
	return &Handler{
		adminApp: adminApp,
	}
}

func (h *Handler) RegisterGroupRoutes(router fiber.Router, _ ...fiber.Handler) {
	router.Get("/", h.adminApp.Guard(integrationroles.PermissionDomainDashboard, integrationroles.PermissionActionRead), h.Dashboard)
	if h.adminApp.Env() == "local" || h.adminApp.Env() == "development" || h.adminApp.Env() == "dev" {
		router.Get("/test/theme", h.adminApp.Guard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionRead), h.TestTheme)     //nolint:lll // required
		router.Get("/test/buttons", h.adminApp.Guard(integrationroles.PermissionDomainAdmins, integrationroles.PermissionActionRead), h.TestButtons) //nolint:lll // required
	}
}

func (h *Handler) Dashboard(c fiber.Ctx) error {
	return h.adminApp.HTTPManager().Render(c, "dashboard/IndexPage", map[string]any{
		titleProp: "Главная",
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
