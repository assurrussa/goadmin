package administrations

import (
	"context"
	"strconv"

	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/internal/builtingrids"
	"github.com/assurrussa/goadmin/models"
)

func (h *Handler) registerWorkflow(router fiber.Router) {
	builtingrids.Register(router, h.adminApp, h.dataGridHandler, "datagrid.admins",
		func(row models.Admin) string { return strconv.FormatInt(row.ID, 10) },
		func(ctx context.Context, ids []string) ([]models.Admin, error) {
			return builtingrids.LoadNumeric(ctx, ids, h.adminRepo.GetByID)
		},
	)
}
