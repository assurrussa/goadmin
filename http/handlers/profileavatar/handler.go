package profileavatar

import (
	"context"
	"net/http"

	logger "github.com/assurrussa/gologger"
	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/gofiber/fiber/v3"

	adminmiddleware "github.com/assurrussa/goadmin/infrastructure/core/middlewares"
	adminpreviewdetach "github.com/assurrussa/goadmin/outbox/preview_detach"
)

type deleteService interface {
	DeleteFile(ctx context.Context, req uploadhost.DeleteRequest) error
}

type Handler struct {
	service deleteService
	logger  logger.Logger
}

func NewHandler(service deleteService, log logger.Logger) *Handler {
	return &Handler{service: service, logger: log}
}

func (h *Handler) RegisterGroupRoutes(route fiber.Router, _ ...fiber.Handler) {
	route.Delete("/auth/profile/avatar", h.Delete)
}

func (h *Handler) Delete(c fiber.Ctx) error {
	admin := adminmiddleware.GetAdminAuth(c)
	if admin == nil || admin.ID <= 0 || admin.UUID.IsZero() {
		return c.Status(http.StatusUnauthorized).JSON(deleteResponse{Status: "failed", Error: "unauthorized"})
	}
	if admin.PreviewID <= 0 {
		return c.JSON(deleteResponse{Status: "deleted"})
	}
	// A job needs the owner and file ID, never the opaque browser credential.
	afterJobs := uploadhost.NewFileEventAfterJobs(adminpreviewdetach.JobName, uploadhost.UserID(admin.UUID))
	if err := h.service.DeleteFile(c, uploadhost.DeleteRequest{
		UserRequestID: uploadhost.UserID(admin.UUID), FileID: admin.PreviewID, AfterJobs: afterJobs,
	}); err != nil {
		h.logger.ErrorContext(c, "failed to enqueue admin avatar deletion", logger.Error(err))
		return c.Status(http.StatusInternalServerError).JSON(deleteResponse{Status: "failed", Error: "failed to delete avatar"})
	}
	return c.Status(http.StatusAccepted).JSON(deleteResponse{Status: "pending", ID: admin.PreviewID})
}

type deleteResponse struct {
	Status string `json:"status"`
	ID     int64  `json:"id,omitempty"`
	Error  string `json:"error,omitempty"`
}
