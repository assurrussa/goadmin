package roles

import (
	"context"
	"strconv"

	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/builtingrids"
	"github.com/assurrussa/goadmin/internal/tablecommands"
)

func (h *Handler) registerWorkflow(router fiber.Router) {
	builtingrids.Register(router, h.adminApp, h.dataGridHandler, "datagrid.roles",
		func(row roleGridItem) string { return strconv.FormatInt(row.ID, 10) },
		func(ctx context.Context, ids []string) ([]roleGridItem, error) {
			wanted := make(map[int64]bool, len(ids))
			for _, raw := range ids {
				if _, err := tablecommands.DecimalID(raw); err != nil {
					return nil, err
				}
				id, _ := strconv.ParseInt(raw, 10, 64)
				wanted[id] = true
			}
			response, err := h.listRoles.Handle(ctx, auth.ListRolesRequest{IncludeSystem: true})
			if err != nil {
				return nil, err
			}
			rows := make([]roleGridItem, 0, len(ids))
			for _, role := range response.Roles {
				if wanted[role.ID] {
					rows = append(rows, roleGridItem{
						ID:   role.ID,
						UUID: role.UUID,
						Slug: role.Slug,
						Name: role.Name,
						Scope: roleScope(role.Slug,
							role.IsSystem),
						IsSystem: role.IsSystem,
					})
				}
			}
			return rows, nil
		},
	)
}
