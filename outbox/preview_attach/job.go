package adminpreviewattach

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	logger3 "github.com/assurrussa/gologger"
	uploadhost "github.com/assurrussa/gouploads/host"

	"github.com/assurrussa/goadmin/infrastructure/outbox"
	identity "github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/internal/uploadintegration"
	"github.com/assurrussa/goadmin/models"
)

//go:generate toolsmocks

const (
	JobName    = "admin_preview_attach"
	loggerName = "job_admin_preview_attach"
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
	logger                       logger3.Logger               `option:"mandatory" validate:"required"`
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
		ctx, payload, func(ctx context.Context, data uploadhost.AfterProcessPayload, file uploadhost.File) error {
			if !uploadintegration.Finalized(file) {
				return errors.New("preview file is not finalized")
			}
			adminModel, err := j.adminRepo.GetByUUID(ctx, identity.UserID(data.UserID))
			if err != nil {
				return fmt.Errorf("get admin model: %w", err)
			}
			expectedPreviewID, hasExpectedPreviewID, err := extractExpectedPreviewID(data.Meta)
			if err != nil {
				return fmt.Errorf("parse expected preview file ID: %w", err)
			}
			if hasExpectedPreviewID && adminModel.GetPreviewFileID() != expectedPreviewID {
				// The preview changed since this upload was queued. Do not overwrite it.
				return nil
			}

			if err := j.adminRepo.UpdatePreview(ctx, adminModel, file.ID); err != nil {
				return fmt.Errorf("update admin model: %w", err)
			}

			if err := j.updateSession(ctx, data, file.ID); err != nil {
				j.logger.WarnContext(ctx, "update admin session preview", logger3.Error(err))
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

func extractExpectedPreviewID(meta map[string]any) (int64, bool, error) {
	value, exists := meta["expectedPreviewFileId"]
	if !exists {
		return 0, false, nil
	}
	valueString, ok := value.(string)
	if !ok {
		return 0, true, fmt.Errorf("expected string, got %T", value)
	}
	previewID, err := strconv.ParseInt(valueString, 10, 64)
	if err != nil {
		return 0, true, err
	}
	if previewID < 0 {
		return 0, true, errors.New("preview file ID must not be negative")
	}
	return previewID, true, nil
}
