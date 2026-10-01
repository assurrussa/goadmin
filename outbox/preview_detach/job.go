package adminpreview

import (
	"context"
	"fmt"

	logger "github.com/assurrussa/gologger"
	uploadhost "github.com/assurrussa/gouploads/host"

	"github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/models"
)

//go:generate toolsmocks

const (
	JobName    = "admin_preview_detach"
	loggerName = "job_admin_preview_detach"
)

type adminRepository interface {
	GetByUUID(ctx context.Context, id identity.UserID) (models.Admin, error)
	UpdatePreview(ctx context.Context, admin models.Admin, fileID int64) error
	UpdateAdmin(ctx context.Context, id int64, admin models.Admin) error
}

type eventFileAfterProcessService interface {
	HandleAfterProcess(ctx context.Context, payload string, fnCall uploadhost.AfterProcessFunc) error
}

type sessionUpdater interface {
	UpdatePreview(ctx context.Context, sessionID string, previewID int64) error
}

//go:generate options-gen -out-filename=job_options.gen.go -from-struct=Options
type Options struct {
	eventFileAfterProcessService eventFileAfterProcessService `option:"mandatory" validate:"required"`
	adminRepo                    adminRepository              `option:"mandatory" validate:"required"`
	sessionUpdater               sessionUpdater               `option:"mandatory" validate:"required"`
	logger                       logger.Logger                `option:"mandatory" validate:"required"`
}

type Job struct {
	outbox.DefaultJob
	Options
}

func Must(opts Options) *Job {
	j, err := New(opts)
	if err != nil {
		panic(err)
	}

	return j
}

func New(opts Options) (*Job, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("validate job options: %w", err)
	}

	opts.logger = opts.logger.WithNamed(loggerName)

	return &Job{
		Options: opts,
	}, nil
}

func (j *Job) Name() string { return JobName }

func (j *Job) Handle(ctx context.Context, payload string) (errReturn error) {
	err := j.eventFileAfterProcessService.HandleAfterProcess(
		ctx, payload, func(ctx context.Context, data uploadhost.AfterProcessPayload, _ uploadhost.File) error {
			adminModel, err := j.adminRepo.GetByUUID(ctx, identity.UserID(data.UserID))
			if err != nil {
				return fmt.Errorf("get admin model: %w", err)
			}
			if data.FileID <= 0 || adminModel.GetPreviewFileID() != data.FileID {
				// A later attach may have replaced the file before this queued
				// detach runs. Do not clear that newer preview or its session state.
				return nil
			}

			if err := j.adminRepo.UpdatePreview(ctx, adminModel, 0); err != nil {
				return fmt.Errorf("update admin model: %w", err)
			}

			if err := j.updateSession(ctx, data, 0); err != nil {
				j.logger.WarnContext(ctx, "update admin session preview", logger.Error(err))
			}

			return nil
		},
	)
	if err != nil {
		return fmt.Errorf("trx: %w", err)
	}

	return nil
}

func (j *Job) updateSession(ctx context.Context, data uploadhost.AfterProcessPayload, previewID int64) error {
	if j.sessionUpdater == nil {
		return nil
	}

	if err := j.sessionUpdater.UpdatePreview(ctx, extractSessionID(data.Meta), previewID); err != nil {
		return fmt.Errorf("update session preview: %w", err)
	}

	return nil
}

func extractSessionID(meta map[string]any) string {
	if len(meta) == 0 {
		return ""
	}

	sessionID, ok := meta["sessionId"].(string)
	if !ok || sessionID == "" {
		return ""
	}

	return sessionID
}
