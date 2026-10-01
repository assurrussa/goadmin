package notificationsjob

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	logger "github.com/assurrussa/gologger"
	notifyjob "github.com/assurrussa/gonotify/interfaces/outbox/notifications"
	coreoutbox "github.com/assurrussa/outbox/outbox"

	"github.com/assurrussa/goadmin/infrastructure/notify"
	"github.com/assurrussa/goadmin/infrastructure/outbox"
	adminnotificationsrepo "github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/adminnotificationrepo"
)

const (
	JobName    = "admin_notifications_dispatch"
	loggerName = "job_admin_notifications_dispatch"
)

//go:generate options-gen -out-filename=job_options.gen.go -from-struct=Options
type Options struct {
	repo     notificationRepository     `option:"mandatory" validate:"required"`
	notifier notify.NotificationManager `option:"mandatory" validate:"required"`
	logger   logger.Logger              `option:"mandatory" validate:"required"`
}

type notificationRepository interface {
	Create(ctx context.Context, params adminnotificationsrepo.CreateParams) (int64, error)
}

type Job struct {
	outbox.DefaultJob
	Options
	delivery *notifyjob.Job
}

func Must(opts Options) *Job {
	job, err := New(opts)
	if err != nil {
		panic(err)
	}

	return job
}

func New(opts Options) (*Job, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	opts.logger = opts.logger.WithNamed(loggerName)
	delivery, err := notifyjob.New(notifyjob.NewOptions(opts.notifier))
	if err != nil {
		return nil, err
	}

	return &Job{
		Options:  opts,
		delivery: delivery,
	}, nil
}

func (j *Job) Name() string { return JobName }

func (j *Job) Handle(ctx context.Context, rawPayload string) error {
	payload, err := UnmarshalPayload(rawPayload)
	if err != nil {
		return coreoutbox.Permanent(fmt.Errorf("unmarshal payload: %w", err))
	}
	if len(payload.Recipients) == 0 {
		return coreoutbox.Permanent(errors.New("payload recipients is empty"))
	}
	if payload.DispatchID == "" {
		jobID := coreoutbox.JobIDFromContext(ctx)
		if jobID.IsZero() {
			return coreoutbox.Permanent(errors.New("notification dispatch id or outbox job identity is required"))
		}
		payload.DispatchID = "job:" + jobID.String()
	}
	requests, err := DeliveryRequests(payload)
	if err != nil {
		return coreoutbox.Permanent(err)
	}

	if err := j.storeNotifications(ctx, payload); err != nil {
		return err
	}

	for _, request := range requests {
		body, err := notifyjob.MarshalPayload(notifyjob.Payload{Request: request})
		if err != nil {
			return coreoutbox.Permanent(err)
		}
		if err := j.delivery.Handle(ctx, body); err != nil {
			j.logger.ErrorContext(ctx, "submit admin notification", logger.Error(err),
				slog.String("idempotency_key", request.IdempotencyKey))
			return err
		}
	}
	return nil
}

func (j *Job) storeNotifications(ctx context.Context, payload Payload) error {
	for _, recipient := range payload.Recipients {
		_, err := j.repo.Create(ctx, adminnotificationsrepo.CreateParams{
			DispatchKey: payload.DispatchID,
			AdminID:     recipient.AdminID,
			Title:       payload.Title,
			Message:     payload.Message,
			Level:       string(payload.Level),
			Payload:     payload.Payload,
		})
		if err != nil {
			return fmt.Errorf("create notification for admin %d: %w", recipient.AdminID, err)
		}
	}

	return nil
}
