package adminnotifications

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	logger "github.com/assurrussa/gologger"
	"github.com/google/uuid"

	"github.com/assurrussa/goadmin/infrastructure/notify"
	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/models"
	notificationsjob "github.com/assurrussa/goadmin/outbox/notifications"
)

type adminRepository interface {
	GetByID(ctx context.Context, id int64) (models.Admin, error)
}

type outboxPutter interface {
	Put(ctx context.Context, name, payload string, availableAt time.Time) (outbox.JobID, error)
}

type Service struct {
	outbox    outboxPutter
	adminRepo adminRepository
	logger    logger.Logger
}

type Request struct {
	AdminIDs  []int64
	Title     string
	Message   string
	Level     notify.NotificationLevel
	Payload   models.NotificationPayload
	Channels  []notify.NotificationChannel
	EmailFrom string
}

func NewService(outbox outboxPutter, adminRepo adminRepository, log logger.Logger) *Service {
	return &Service{
		outbox:    outbox,
		adminRepo: adminRepo,
		logger:    log,
	}
}

func (s *Service) Enqueue(ctx context.Context, req Request) (outbox.JobID, error) {
	if s.outbox == nil {
		return outbox.JobIDNil, errors.New("outbox service is nil")
	}
	if len(req.AdminIDs) == 0 {
		return outbox.JobIDNil, errors.New("admin ids list is empty")
	}
	if strings.TrimSpace(req.Title) == "" {
		return outbox.JobIDNil, errors.New("title is required")
	}
	if strings.TrimSpace(req.Message) == "" {
		return outbox.JobIDNil, errors.New("message is required")
	}
	if req.Payload == nil {
		req.Payload = models.NotificationPayload{}
	}
	if s.adminRepo == nil {
		return outbox.JobIDNil, errors.New("admin repository is nil")
	}

	level := req.Level
	if level == "" {
		level = notify.LevelInfo
	}

	recipients := make([]notificationsjob.Recipient, 0, len(req.AdminIDs))
	seen := make(map[int64]bool, len(req.AdminIDs))
	for _, adminID := range req.AdminIDs {
		if seen[adminID] {
			continue
		}
		seen[adminID] = true
		adminModel, err := s.adminRepo.GetByID(ctx, adminID)
		if err != nil {
			s.logger.ErrorContext(ctx, "admin notifications: load admin", logger.Error(err))
			continue
		}
		if adminModel.ID == 0 {
			continue
		}
		recipients = append(recipients, notificationsjob.Recipient{
			AdminID: adminModel.ID,
			Name:    fmt.Sprintf("%s %s", adminModel.Name, adminModel.LastName),
			Email:   adminModel.Email,
		})
	}

	if len(recipients) == 0 {
		return outbox.JobIDNil, errors.New("no valid recipients")
	}

	payload := notificationsjob.Payload{
		DispatchID: uuid.NewString(),
		EmailFrom:  req.EmailFrom,
		Title:      req.Title,
		Message:    req.Message,
		Level:      level,
		Payload:    req.Payload,
		Channels:   req.Channels,
		Recipients: recipients,
	}
	if _, err := notificationsjob.DeliveryRequests(payload); err != nil {
		return outbox.JobIDNil, fmt.Errorf("validate delivery request: %w", err)
	}

	body, err := notificationsjob.MarshalPayload(payload)
	if err != nil {
		return outbox.JobIDNil, fmt.Errorf("marshal payload: %w", err)
	}

	jobID, err := s.outbox.Put(ctx, notificationsjob.JobName, body, time.Now())
	if err != nil {
		return outbox.JobIDNil, fmt.Errorf("enqueue admin notification: %w", err)
	}

	return jobID, nil
}
