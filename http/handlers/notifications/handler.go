package notifications

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	logger "github.com/assurrussa/gologger"
	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/adminapp"
	goinertia "github.com/assurrussa/goadmin/infrastructure/inertia"
	adminnotifications "github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminnotificationrepo"
	"github.com/assurrussa/goadmin/shared"
)

const (
	defaultPerPage = 10
	maxPerPage     = 50
	minPerPage     = 5
)

type notificationRepository interface {
	ListByAdmin(ctx context.Context, params adminnotifications.ListParams) (adminnotifications.ListResult, error)
	MarkRead(ctx context.Context, adminID, notificationID int64) error
	MarkAllRead(ctx context.Context, adminID int64) (int64, error)
}

type Handler struct {
	adminApp  *adminapp.App
	repo      notificationRepository
	routePath string
	logger    logger.Logger
}

func NewHandler(adminApp *adminapp.App, repo notificationRepository) *Handler {
	return &Handler{
		adminApp:  adminApp,
		repo:      repo,
		routePath: "/notifications",
		logger:    adminApp.Logger(),
	}
}

func (h *Handler) RegisterGroupRoutes(route fiber.Router, _ ...fiber.Handler) {
	group := route.Group(h.routePath)
	group.Get("", h.Index)
	group.Post("read-all", h.MarkAllRead)
	group.Post(":id/read", h.MarkRead)
}

func (h *Handler) Index(c fiber.Ctx) error {
	admin := shared.MustGetAdminAuth(c)

	page := parsePositiveInt(c.Query("page"), 1)
	perPage := clampPerPage(parsePositiveInt(c.Query("perPage"), defaultPerPage))
	status := normalizeStatus(c.Query("status"))

	offset := uint64((page - 1) * perPage)

	result, err := h.repo.ListByAdmin(c, adminnotifications.ListParams{
		AdminID: admin.ID,
		Status:  status,
		Limit:   uint64(perPage),
		Offset:  offset,
	})
	if err != nil {
		h.logger.ErrorContext(c, "list admin notifications", logger.Error(err))
		return goinertia.NewError(fiber.StatusInternalServerError, "Не удалось загрузить уведомления", err)
	}

	totalPages := 0
	if perPage > 0 && result.Total > 0 {
		totalPages = (result.Total + perPage - 1) / perPage
	}

	props := map[string]any{
		"title":         "Уведомления",
		"notifications": result.Notifications,
		"meta": map[string]any{
			"total":       result.Total,
			"unreadCount": result.UnreadCount,
			"page":        page,
			"perPage":     perPage,
			"totalPages":  totalPages,
			"hasMore":     page < totalPages,
		},
		"filters": map[string]any{
			"status":  status,
			"page":    page,
			"perPage": perPage,
		},
	}

	return h.adminApp.HTTPManager().Render(c, "notifications/IndexPage", props)
}

func (h *Handler) MarkRead(c fiber.Ctx) error {
	admin := shared.MustGetAdminAuth(c)
	notificationID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || notificationID <= 0 {
		return goinertia.NewError(fiber.StatusBadRequest, "Некорректный идентификатор уведомления", err)
	}

	if err := h.repo.MarkRead(c, admin.ID, notificationID); err != nil {
		h.logger.ErrorContext(
			c,
			"mark notification read",
			logger.Error(err),
			slog.Int64("admin_id", admin.ID),
			slog.Int64("notification_id", notificationID),
		)
		h.adminApp.HTTPManager().WithFlashError(c, "Не удалось отметить уведомление прочитанным")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	h.adminApp.HTTPManager().WithFlashSuccess(c, "Уведомление отмечено прочитанным")
	return h.adminApp.HTTPManager().RedirectBack(c)
}

func (h *Handler) MarkAllRead(c fiber.Ctx) error {
	admin := shared.MustGetAdminAuth(c)

	if _, err := h.repo.MarkAllRead(c, admin.ID); err != nil {
		h.logger.ErrorContext(c, "mark all notifications read", logger.Error(err), slog.Int64("admin_id", admin.ID))
		h.adminApp.HTTPManager().WithFlashError(c, "Не удалось отметить уведомления прочитанными")
		return h.adminApp.HTTPManager().RedirectBack(c)
	}

	h.adminApp.HTTPManager().WithFlashSuccess(c, "Все уведомления отмечены прочитанными")
	return h.adminApp.HTTPManager().RedirectBack(c)
}

func clampPerPage(value int) int {
	if value < minPerPage {
		return minPerPage
	}
	if value > maxPerPage {
		return maxPerPage
	}
	return value
}

func parsePositiveInt(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}

	return value
}

func normalizeStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case adminnotifications.StatusUnread:
		return adminnotifications.StatusUnread
	default:
		return adminnotifications.StatusAll
	}
}
