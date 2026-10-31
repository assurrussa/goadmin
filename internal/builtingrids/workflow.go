// Package builtingrids connects built-in tables to optional personal workflows.
package builtingrids

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/adminapp"
	"github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/gridworkflow"
	"github.com/assurrussa/goadmin/internal/tablecommands"
	"github.com/assurrussa/goadmin/shared"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

func Register[T any](router fiber.Router, app *adminapp.App, grid *datagrid.Handler[T], key string,
	id func(T) string, selected func(context.Context, []string) ([]T, error), actions ...datagrid.WorkflowAction,
) {
	if app.GridDB() == nil {
		return
	}
	bulkURL := ""
	if len(actions) > 0 {
		bulkURL = grid.GetConfig().RoutePath + "/bulk"
	}
	err := gridworkflow.Register(router, grid, gridworkflow.Options[T]{
		Key: key, Store: gridworkflow.NewPostgresStore(app.GridDB()), SelectionID: id, LoadSelected: selected,
		BulkURL: bulkURL, BulkActions: actions,
		CurrentAdmin: func(c fiber.Ctx) (int64, error) {
			actor := shared.GetAdminAuth(c)
			if actor == nil {
				return 0, fiber.ErrUnauthorized
			}
			return actor.ID, nil
		},
	})
	if err != nil {
		panic(fmt.Errorf("configure built-in table %s: %w", key, err))
	}
}

func Action(app *adminapp.App, domain auth.PermissionDomain, permission auth.PermissionAction,
	key, label string,
) datagrid.WorkflowAction {
	return datagrid.WorkflowAction{Key: key, Label: label, CanView: func(ctx context.Context) bool {
		actor := shared.GetAdminAuth(ctx)
		return actor != nil && app.RolesService().AdminCan(ctx, actor.ID, domain, permission)
	}}
}

func LoadNumeric[T any](ctx context.Context, ids []string, load func(context.Context, int64) (T, error)) ([]T, error) {
	rows := make([]T, 0, len(ids))
	for _, raw := range ids {
		if _, err := tablecommands.DecimalID(raw); err != nil {
			return nil, err
		}
		id, _ := strconv.ParseInt(raw, 10, 64)
		row, err := load(ctx, id)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}
