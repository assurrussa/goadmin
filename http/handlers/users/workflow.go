package users

import (
	"context"
	"strconv"

	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/builtingrids"
)

func (h *Handler) registerWorkflow(router fiber.Router) {
	builtingrids.Register(router, h.adminApp, h.dataGridHandler, "datagrid.users",
		func(row auth.Profile) string { return strconv.FormatInt(row.ID, 10) },
		func(ctx context.Context, ids []string) ([]auth.Profile, error) {
			return builtingrids.LoadNumeric(ctx, ids, h.userRepo.GetByID)
		},
		builtingrids.Action(h.adminApp, auth.PermissionDomainUsers, auth.PermissionActionDelete, userDeleteAction, "Удалить"),
		builtingrids.Action(h.adminApp, auth.PermissionDomainUsers, auth.PermissionActionDelete, userRestoreAction, "Восстановить"),
	)
}
