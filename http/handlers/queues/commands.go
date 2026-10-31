package queues

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminactionauditrepo"
	"github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/internal/tablecommands"
)

const (
	queueDeleteAction = "delete"
	queueRetryAction  = "retry"
)

func (h *Handler) deleteJob(ctx context.Context, id outbox.JobID, record adminactionauditrepo.Record) error {
	if h.auditWriter == nil {
		return errors.New("admin action audit writer is required")
	}
	return h.adminApp.TxManager().RunInTx(ctx, func(txCtx context.Context) error {
		affected, err := h.jobsRepo.DeleteJob(txCtx, id)
		if err != nil {
			return err
		}
		if affected != 1 {
			return outbox.ErrNoJobs
		}
		return h.auditWriter.RecordAdminAction(txCtx, record)
	})
}

func (h *Handler) deleteFailedJob(ctx context.Context, id outbox.JobID, record adminactionauditrepo.Record) error {
	if h.auditWriter == nil {
		return errors.New("admin action audit writer is required")
	}
	return h.adminApp.TxManager().RunInTx(ctx, func(txCtx context.Context) error {
		affected, err := h.jobsFailedRepo.Delete(txCtx, id)
		if err != nil {
			return err
		}
		if affected != 1 {
			return outbox.ErrNoJobs
		}
		return h.auditWriter.RecordAdminAction(txCtx, record)
	})
}

func (h *Handler) retryFailedJob(ctx context.Context, id outbox.JobID, record adminactionauditrepo.Record) (string, error) {
	if h.auditWriter == nil {
		return "", errors.New("admin action audit writer is required")
	}
	var related string
	err := h.adminApp.TxManager().RunInTx(ctx, func(txCtx context.Context) error {
		failedJob, err := h.jobsFailedRepo.GetByID(txCtx, id)
		if err != nil {
			return err
		}
		affected, err := h.jobsFailedRepo.Delete(txCtx, id)
		if err != nil {
			return err
		}
		if affected != 1 {
			return outbox.ErrNoJobs
		}
		now := time.Now()
		newID, err := h.jobsRepo.Create(txCtx, outbox.JobModel{
			ID: outbox.NewJobID(), Queue: ensureQueue(failedJob.Queue), Name: failedJob.Name,
			SchemaVersion: failedJob.SchemaVersion, Payload: failedJob.Payload, Attempts: 0,
			ReservedAt: sql.NullTime{}, AvailableAt: now, CreatedAt: now,
		})
		if err != nil {
			return err
		}
		related = newID.String()
		record.RelatedTargetID = related
		return h.auditWriter.RecordAdminAction(txCtx, record)
	})
	if err != nil {
		return "", err
	}
	return related, nil
}

func queueID(raw string) (string, error) {
	id, err := outbox.Parse[outbox.JobID](raw)
	if err != nil || id.IsZero() {
		return "", fiber.ErrBadRequest
	}
	return id.String(), nil
}

func (h *Handler) BulkJobs(c fiber.Ctx) error {
	request, err := tablecommands.Parse(c.Body(), []string{queueDeleteAction}, queueID)
	if err != nil {
		return err
	}
	return tablecommands.Run(c, request, func(raw string) (string, error) {
		id, _ := outbox.Parse[outbox.JobID](raw)
		err := h.deleteJob(c, id, h.newAuditRecord(c, adminactionauditrepo.ActionQueueJobDeleted, "queue_job", raw))
		if errors.Is(err, outbox.ErrNoJobs) {
			return "", tablecommands.ErrSkipped
		}
		return "", err
	})
}

func (h *Handler) bulkFailedGuard(c fiber.Ctx) error {
	var request tablecommands.Request
	if len(c.Body()) > 64*1024 || json.Unmarshal(c.Body(), &request) != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Invalid bulk command"})
	}
	action := auth.PermissionActionDelete
	if request.Action == queueRetryAction {
		action = auth.PermissionActionUpdate
	} else if request.Action != queueDeleteAction {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Invalid bulk command"})
	}
	return h.adminApp.Guard(auth.PermissionDomainQueues, action)(c)
}

func (h *Handler) BulkFailedJobs(c fiber.Ctx) error {
	request, err := tablecommands.Parse(c.Body(), []string{queueDeleteAction, queueRetryAction}, queueID)
	if err != nil {
		return err
	}
	return tablecommands.Run(c, request, func(raw string) (string, error) {
		id, _ := outbox.Parse[outbox.JobID](raw)
		if request.Action == queueRetryAction {
			record := h.newAuditRecord(c, adminactionauditrepo.ActionQueueFailedRetry, "queue_failed_job", raw)
			related, err := h.retryFailedJob(c, id, record)
			if errors.Is(err, outbox.ErrNoJobs) {
				return "", tablecommands.ErrSkipped
			}
			return related, err
		}
		err := h.deleteFailedJob(c, id, h.newAuditRecord(c, adminactionauditrepo.ActionQueueFailedDelete, "queue_failed_job", raw))
		if errors.Is(err, outbox.ErrNoJobs) {
			return "", tablecommands.ErrSkipped
		}
		return "", err
	})
}
