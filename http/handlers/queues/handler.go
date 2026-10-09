package queues

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"

	"github.com/assurrussa/goadmin/adminapp"
	datagrid "github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminactionauditrepo"
	integrationroles "github.com/assurrussa/goadmin/internal/auth"
	"github.com/assurrussa/goadmin/shared"
)

//go:generate toolsmocks

type jobsRepository interface {
	datagrid.Repository[outbox.JobModel]
	DeleteJob(ctx context.Context, jobID outbox.JobID) (int64, error)
	CountExact(ctx context.Context) (int64, error)
	Create(ctx context.Context, job outbox.JobModel) (outbox.JobID, error)
}

type jobsFailedRepository interface {
	datagrid.Repository[outbox.JobFailedModel]
	Delete(ctx context.Context, jobID outbox.JobID) (int64, error)
	GetByID(ctx context.Context, id outbox.JobID) (outbox.JobFailedModel, error)
	CountExact(ctx context.Context) (int64, error)
}

type Handler struct {
	adminApp       *adminapp.App
	routePath      string
	jobsRoute      string
	jobsFailed     string
	jobsGrid       *datagrid.Handler[outbox.JobModel]
	jobsFailedGrid *datagrid.Handler[outbox.JobFailedModel]
	jobsRepo       jobsRepository
	jobsFailedRepo jobsFailedRepository
	auditWriter    adminactionauditrepo.Writer
}

func NewHandler(
	adminApp *adminapp.App,
	jobsRepo jobsRepository,
	jobsFailedRepo jobsFailedRepository,
	auditWriter adminactionauditrepo.Writer,
) *Handler {
	routePath := "/queues"
	jobsRoute := routePath + "/jobs"
	jobsFailedRoute := routePath + "/jobs-failed"

	jobsGrid := datagrid.NewHandler(datagrid.FilterConfig[outbox.JobModel]{
		RoutePath:        jobsRoute,
		Entity:           "jobs",
		Title:            "Очередь задач",
		Repository:       jobsRepo,
		DefaultSort:      "createdAt", //nolint:goconst // required
		DefaultOrder:     "desc",
		PageSize:         25,
		Refreshable:      true,
		SearchMode:       datagrid.SearchModeNone,
		CreateButtonText: "",
		EmptyMessage:     "В очереди сейчас нет задач",
		Columns: []datagrid.Column{
			{Key: "id", Label: "ID", Type: "number", Sortable: true},                         //nolint:goconst // required
			{Key: "queue", Label: "Очередь", Type: "text", Sortable: true, Filterable: true}, //nolint:goconst // required
			{Key: "name", Label: "Задача", Type: "text", Sortable: true, Filterable: true},
			{Key: "attempts", Label: "Попытки", Type: "number", Sortable: true},
			{Key: "reservedAt", Label: "Зарезервировано", Type: "date", Format: "2006-01-02 15:04", Sortable: true}, //nolint:goconst,lll // required
			{Key: "availableAt", Label: "Доступно", Type: "date", Format: "2006-01-02 15:04", Sortable: true},
			{Key: "createdAt", Label: "Создано", Type: "date", Format: "2006-01-02 15:04", Sortable: true},
			{Key: "payloadPreview", Label: "Payload", Type: "text"}, //nolint:goconst // required
		},
		Actions: []datagrid.Action[outbox.JobModel]{
			{Key: "delete", Label: "Удалить", Icon: "trash", Variant: "danger"},
		},
		RowDecorators: []datagrid.RowDecorator[outbox.JobModel]{
			datagrid.NewRowDecorator(
				func(_ context.Context, jobs []outbox.JobModel) error {
					for i := range jobs {
						jobs[i].Payload = maskPayload(jobs[i].Payload)
					}

					return nil
				},
				func(_ context.Context, job outbox.JobModel) (map[string]any, error) {
					reserved := any(nil)
					if job.ReservedAt.Valid {
						reserved = job.ReservedAt.Time
					}

					return map[string]any{
						"reservedAt":     reserved,
						"payloadPreview": maskPayload(job.Payload),
					}, nil
				},
			),
		},
	}, adminApp.Logger())

	jobsFailedGrid := datagrid.NewHandler(datagrid.FilterConfig[outbox.JobFailedModel]{
		RoutePath:    jobsFailedRoute,
		Entity:       "jobs_failed",
		Title:        "Проблемные задачи",
		Repository:   jobsFailedRepo,
		DefaultSort:  "failedAt",
		DefaultOrder: "desc",
		PageSize:     25,
		Refreshable:  true,
		SearchMode:   datagrid.SearchModeNone,
		EmptyMessage: "Ошибок в очереди нет",
		Columns: []datagrid.Column{
			{Key: "id", Label: "ID", Type: "number", Sortable: true},
			{Key: "name", Label: "Задача", Type: "text", Sortable: true, Filterable: true},
			{Key: "queue", Label: "Очередь", Type: "text", Sortable: true},
			{Key: "reason", Label: "Причина", Type: "text"},
			{Key: "exceptionPreview", Label: "Ошибка", Type: "text"},
			{Key: "failedAt", Label: "Провалено", Type: "date", Format: "2006-01-02 15:04", Sortable: true},
			{Key: "createdAt", Label: "Создано", Type: "date", Format: "2006-01-02 15:04", Sortable: true},
			{Key: "payloadPreview", Label: "Payload", Type: "text"},
		},
		Actions: []datagrid.Action[outbox.JobFailedModel]{
			{Key: "retry", Label: "Повторить", Icon: "arrow-path", Variant: "primary"},
			{Key: "delete", Label: "Удалить", Icon: "trash", Variant: "danger"},
		},
		RowDecorators: []datagrid.RowDecorator[outbox.JobFailedModel]{
			datagrid.NewRowDecorator(
				func(_ context.Context, jobs []outbox.JobFailedModel) error {
					for i := range jobs {
						jobs[i].Payload = maskPayload(jobs[i].Payload)
					}

					return nil
				},
				func(_ context.Context, job outbox.JobFailedModel) (map[string]any, error) {
					values := map[string]any{
						"payloadPreview":   maskPayload(job.Payload),
						"exceptionPreview": shorten(job.Exception, 200),
					}

					return values, nil
				},
			),
		},
	}, adminApp.Logger())

	return &Handler{
		adminApp:       adminApp,
		routePath:      routePath,
		jobsRoute:      jobsRoute,
		jobsFailed:     jobsFailedRoute,
		jobsGrid:       jobsGrid,
		jobsFailedGrid: jobsFailedGrid,
		jobsRepo:       jobsRepo,
		jobsFailedRepo: jobsFailedRepo,
		auditWriter:    auditWriter,
	}
}

func (h *Handler) RegisterGroupRoutes(route fiber.Router, _ ...fiber.Handler) {
	guarded := route.Group(h.routePath, h.adminApp.Guard(integrationroles.PermissionDomainQueues, integrationroles.PermissionActionRead)) //nolint:lll // required

	guarded.Get("", h.Index)
	guarded.Get("/jobs", h.JobsPage)
	guarded.Get("/jobs-failed", h.FailedPage)

	jobs := guarded.Group("/jobs")
	jobs.Get("/data", h.jobsGrid.HandleData)
	jobs.Post("/data", h.jobsGrid.HandleData)
	jobs.Delete(":id", h.adminApp.Guard(integrationroles.PermissionDomainQueues, integrationroles.PermissionActionDelete), h.DeleteJob) //nolint:lll // required

	failed := guarded.Group("/jobs-failed")
	failed.Get("/data", h.jobsFailedGrid.HandleData)
	failed.Post("/data", h.jobsFailedGrid.HandleData)
	failed.Delete(
		":id",
		h.adminApp.Guard(integrationroles.PermissionDomainQueues, integrationroles.PermissionActionDelete),
		h.DeleteFailedJob,
	)
	failed.Post(
		":id/retry",
		h.adminApp.Guard(integrationroles.PermissionDomainQueues, integrationroles.PermissionActionUpdate),
		h.RetryFailedJob,
	)
}

func (h *Handler) Index(c fiber.Ctx) error {
	jobsCount, failedCount, err := h.loadStats(c)
	if err != nil {
		return err
	}

	return h.adminApp.HTTPManager().Render(c, "queues/IndexPage", map[string]any{
		"title":        "Очереди задач", //nolint:goconst // required
		"jobsApiUrl":   h.jobsRoute,
		"failedApiUrl": h.jobsFailed,
		"stats": map[string]any{
			"jobs":   jobsCount,
			"failed": failedCount,
		},
	})
}

func (h *Handler) JobsPage(c fiber.Ctx) error {
	jobsCount, failedCount, err := h.loadStats(c)
	if err != nil {
		return err
	}

	return h.adminApp.HTTPManager().Render(c, "queues/ActivePage", map[string]any{
		"title":       "Активные задачи",
		"jobsApiUrl":  h.jobsRoute,
		"jobsCount":   jobsCount,
		"failedCount": failedCount,
	})
}

func (h *Handler) FailedPage(c fiber.Ctx) error {
	jobsCount, failedCount, err := h.loadStats(c)
	if err != nil {
		return err
	}

	return h.adminApp.HTTPManager().Render(c, "queues/FailedPage", map[string]any{
		"title":        "DLQ / Ошибки",
		"failedApiUrl": h.jobsFailed,
		"jobsCount":    jobsCount,
		"failedCount":  failedCount,
	})
}

func (h *Handler) DeleteJob(c fiber.Ctx) error {
	id, err := h.getID(c)
	if err != nil {
		return err
	}
	if h.auditWriter == nil {
		return errors.New("admin action audit writer is required")
	}
	record := h.newAuditRecord(c, adminactionauditrepo.ActionQueueJobDeleted, "queue_job", id.String(), "")
	err = h.adminApp.TxManager().RunInTx(c, func(ctx context.Context) error {
		affected, err := h.jobsRepo.DeleteJob(ctx, id)
		if err != nil {
			return fmt.Errorf("delete job %d: %w", id, err)
		}
		if affected != 1 {
			return outbox.ErrNoJobs
		}
		return h.auditWriter.RecordAdminAction(ctx, record)
	})
	if errors.Is(err, outbox.ErrNoJobs) {
		return fiber.NewError(fiber.StatusNotFound, "Задача не найдена")
	}
	if err != nil {
		return fmt.Errorf("delete job %d: %w", id, err)
	}

	h.adminApp.HTTPManager().WithFlashSuccess(c, "Задача удалена из очереди")
	return h.adminApp.HTTPManager().RedirectBack(c)
}

func (h *Handler) DeleteFailedJob(c fiber.Ctx) error {
	id, err := h.getID(c)
	if err != nil {
		return err
	}

	if h.auditWriter == nil {
		return errors.New("admin action audit writer is required")
	}
	record := h.newAuditRecord(c, adminactionauditrepo.ActionQueueFailedDelete, "queue_failed_job", id.String(), "")
	err = h.adminApp.TxManager().RunInTx(c, func(ctx context.Context) error {
		affected, err := h.jobsFailedRepo.Delete(ctx, id)
		if err != nil {
			return fmt.Errorf("delete failed job %d: %w", id, err)
		}
		if affected != 1 {
			return outbox.ErrNoJobs
		}
		return h.auditWriter.RecordAdminAction(ctx, record)
	})
	if errors.Is(err, outbox.ErrNoJobs) {
		return fiber.NewError(fiber.StatusNotFound, "Запись не найдена в DLQ")
	}
	if err != nil {
		return fmt.Errorf("delete failed job %d: %w", id, err)
	}

	h.adminApp.HTTPManager().WithFlashSuccess(c, "Задача удалена из очереди ошибок")
	return h.adminApp.HTTPManager().RedirectBack(c)
}

func (h *Handler) RetryFailedJob(c fiber.Ctx) error {
	id, err := h.getID(c)
	if err != nil {
		return err
	}
	if h.auditWriter == nil {
		return errors.New("admin action audit writer is required")
	}
	record := h.newAuditRecord(c, adminactionauditrepo.ActionQueueFailedRetry, "queue_failed_job", id.String(), "")

	availableAt := time.Now()

	err = h.adminApp.TxManager().RunInTx(c, func(ctx context.Context) error {
		failedJob, err := h.jobsFailedRepo.GetByID(ctx, id)
		if err != nil {
			return fmt.Errorf("load failed job %d: %w", id, err)
		}

		affected, err := h.jobsFailedRepo.Delete(ctx, id)
		if err != nil {
			return fmt.Errorf("cleanup failed job: %w", err)
		}
		if affected != 1 {
			return outbox.ErrNoJobs
		}

		newJobID := outbox.NewJobID()
		_, err = h.jobsRepo.Create(ctx, outbox.JobModel{
			ID:            newJobID,
			Queue:         ensureQueue(failedJob.Queue),
			Name:          failedJob.Name,
			SchemaVersion: failedJob.SchemaVersion,
			Payload:       failedJob.Payload,
			Attempts:      0,
			ReservedAt:    sql.NullTime{},
			AvailableAt:   availableAt,
			CreatedAt:     availableAt,
		})
		if err != nil {
			return fmt.Errorf("create retry job: %w", err)
		}

		record.RelatedTargetID = newJobID.String()
		return h.auditWriter.RecordAdminAction(ctx, record)
	})
	if err != nil {
		if errors.Is(err, outbox.ErrNoJobs) {
			return fiber.NewError(fiber.StatusNotFound, "Задача не найдена в DLQ")
		}
		return fmt.Errorf("retry failed job %d: %w", id, err)
	}

	h.adminApp.HTTPManager().WithFlashSuccess(c, "Задача отправлена на повторную обработку")
	return h.adminApp.HTTPManager().RedirectBack(c)
}

func (h *Handler) newAuditRecord(
	c fiber.Ctx,
	action string,
	targetType string,
	targetID string,
	relatedTargetID string,
) adminactionauditrepo.Record {
	actor := shared.MustGetAdminAuth(c)
	return adminactionauditrepo.Record{
		ActorAdminID:    actor.ID,
		ActorSubjectID:  actor.AuthSubjectID().String(),
		Action:          action,
		TargetType:      targetType,
		TargetID:        targetID,
		RelatedTargetID: relatedTargetID,
		RequestID:       requestid.FromContext(c),
	}
}

func (h *Handler) getID(c fiber.Ctx) (outbox.JobID, error) {
	id := c.Params("id")
	if id == "" {
		return outbox.JobIDNil, fiber.NewError(fiber.StatusBadRequest, "Некорректный идентификатор")
	}

	return outbox.Parse[outbox.JobID](id)
}

func (h *Handler) loadStats(ctx context.Context) (jobsCount int64, failedCount int64, errReturn error) {
	var err error
	jobsCount, err = h.jobsRepo.CountExact(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("jobs queues: count active: %w", err)
	}

	failedCount, err = h.jobsFailedRepo.CountExact(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("jobs queues: count failed: %w", err)
	}

	return jobsCount, failedCount, nil
}

func shorten(value string, limit int) string {
	if value == "" || len(value) <= limit {
		return value
	}

	if limit <= 3 {
		return value
	}

	return value[:limit-3] + "..."
}

func maskPayload(value string) string {
	if value == "" {
		return ""
	}

	return "Скрыто"
}

func ensureQueue(queue string) string {
	if queue == "" {
		return "queue"
	}

	return queue
}
