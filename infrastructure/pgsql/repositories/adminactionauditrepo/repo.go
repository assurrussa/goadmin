package adminactionauditrepo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/internal/admintx"
)

const tableName = "admin_action_audit"

const (
	ActionAdminSoftDeleted  = "admin.soft_deleted"
	ActionQueueJobDeleted   = "queue.job_deleted"
	ActionQueueFailedDelete = "queue.failed_job_deleted"
	ActionQueueFailedRetry  = "queue.failed_job_retried"
)

type Record struct {
	ActorAdminID    int64
	ActorSubjectID  string
	Action          string
	TargetType      string
	TargetID        string
	RelatedTargetID string
	RequestID       string
	OccurredAt      time.Time
}

type Writer interface {
	RecordAdminAction(ctx context.Context, record Record) error
}

//go:generate options-gen -out-filename=repo_options.gen.go -from-struct=Options
type Options struct {
	pgsql outbox.StoragePgsqlClient `option:"mandatory" validate:"required"`
}

type Repo struct{ Options }

func New(opts Options) (*Repo, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("validate admin action audit repo options: %w", err)
	}

	return &Repo{Options: opts}, nil
}

func Must(opts Options) *Repo {
	repo, err := New(opts)
	if err != nil {
		panic(fmt.Errorf("admin action audit repo: %w", err))
	}

	return repo
}

func (r *Repo) RecordAdminAction(ctx context.Context, record Record) error {
	const op = "adminactionaudit.repo.RecordAdminAction"
	if record.ActorAdminID <= 0 || strings.TrimSpace(record.Action) == "" ||
		strings.TrimSpace(record.TargetType) == "" || strings.TrimSpace(record.TargetID) == "" {
		return fmt.Errorf("%s: invalid audit record", op)
	}
	if record.OccurredAt.IsZero() {
		record.OccurredAt = time.Now().UTC()
	} else {
		record.OccurredAt = record.OccurredAt.UTC()
	}

	builder := outbox.BuilderDollar().
		Insert(tableName).
		Columns(
			"actor_admin_id", "actor_subject_id", "action", "target_type", "target_id",
			"related_target_id", "request_id", "occurred_at",
		).
		Values(record.ActorAdminID, nullableString(record.ActorSubjectID), record.Action, record.TargetType,
			record.TargetID, nullableString(record.RelatedTargetID), nullableString(record.RequestID), record.OccurredAt)

	if _, err := admintx.Wrap(r.pgsql.DB()).Execx(ctx, op, builder); err != nil {
		return fmt.Errorf("%s: %w", op, outbox.ErrorTransform(err))
	}

	return nil
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

var _ Writer = (*Repo)(nil)
