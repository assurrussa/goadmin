package queues

import (
	"context"

	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/builtingrids"
)

func (h *Handler) registerJobsWorkflow(router fiber.Router) {
	builtingrids.Register(router, h.adminApp, h.jobsGrid, "datagrid.jobs",
		func(row outbox.JobModel) string { return row.ID.String() },
		func(ctx context.Context, ids []string) ([]outbox.JobModel, error) {
			loader, ok := h.jobsRepo.(interface {
				GetByID(ctx context.Context, id outbox.JobID) (outbox.JobModel, error)
			})
			if !ok {
				return nil, fiber.ErrServiceUnavailable
			}
			rows := make([]outbox.JobModel, 0, len(ids))
			for _, raw := range ids {
				id, err := outbox.Parse[outbox.JobID](raw)
				if err != nil {
					return nil, fiber.ErrBadRequest
				}
				row, err := loader.GetByID(ctx, id)
				if err != nil {
					return nil, err
				}
				rows = append(rows, row)
			}
			return rows, nil
		},
		builtingrids.Action(h.adminApp, auth.PermissionDomainQueues, auth.PermissionActionDelete, queueDeleteAction, "Удалить"),
	)
}

func (h *Handler) registerFailedWorkflow(router fiber.Router) {
	builtingrids.Register(router, h.adminApp, h.jobsFailedGrid, "datagrid.jobs_failed",
		func(row outbox.JobFailedModel) string { return row.ID.String() },
		func(ctx context.Context, ids []string) ([]outbox.JobFailedModel, error) {
			rows := make([]outbox.JobFailedModel, 0, len(ids))
			for _, raw := range ids {
				id, err := outbox.Parse[outbox.JobID](raw)
				if err != nil {
					return nil, fiber.ErrBadRequest
				}
				row, err := h.jobsFailedRepo.GetByID(ctx, id)
				if err != nil {
					return nil, err
				}
				rows = append(rows, row)
			}
			return rows, nil
		},
		builtingrids.Action(h.adminApp, auth.PermissionDomainQueues, auth.PermissionActionUpdate, queueRetryAction, "Повторить"),
		builtingrids.Action(h.adminApp, auth.PermissionDomainQueues, auth.PermissionActionDelete, queueDeleteAction, "Удалить"),
	)
}
