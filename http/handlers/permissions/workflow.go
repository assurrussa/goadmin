package permissions

import (
	"context"
	"strconv"

	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/builtingrids"
	"github.com/assurrussa/goadmin/internal/tablecommands"
)

func (h *Handler) registerWorkflow(router fiber.Router) {
	builtingrids.Register(router, h.adminApp, h.dataGridHandler, "datagrid.permissions",
		func(row permissionGridItem) string { return strconv.FormatInt(row.ID, 10) },
		func(ctx context.Context, ids []string) ([]permissionGridItem, error) {
			wanted := make(map[int64]bool, len(ids))
			for _, raw := range ids {
				if _, err := tablecommands.DecimalID(raw); err != nil {
					return nil, err
				}
				id, _ := strconv.ParseInt(raw, 10, 64)
				wanted[id] = true
			}
			response, err := h.listPermissions.Handle(ctx, auth.ListPermissionsRequest{})
			if err != nil {
				return nil, err
			}
			rows := make([]permissionGridItem, 0, len(ids))
			for _, p := range response.Permissions {
				if wanted[p.ID] {
					rows = append(rows, permissionGridItem{
						ID:          p.ID,
						UUID:        p.UUID,
						Domain:      p.Domain,
						Action:      p.Action,
						Description: p.Description,
					})
				}
			}
			return rows, nil
		},
	)
}
