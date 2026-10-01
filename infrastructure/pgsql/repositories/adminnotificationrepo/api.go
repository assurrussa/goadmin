package adminnotificationrepo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Masterminds/squirrel"
	querybuilder "github.com/assurrussa/outbox/shared/query_builder"
	"github.com/georgysavva/scany/v2/pgxscan"

	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/models"
)

const tableName = "admin_notifications"

const (
	StatusAll    = "all"
	StatusUnread = "unread"
)

type ListParams struct {
	AdminID int64
	Status  string
	Limit   uint64
	Offset  uint64
}

type ListResult struct {
	Notifications []models.AdminNotification
	Total         int
	UnreadCount   int
}

type CreateParams struct {
	DispatchKey string
	AdminID     int64
	Title       string
	Message     string
	Level       string
	Payload     models.NotificationPayload
}

func (r *Repo) ListByAdmin(ctx context.Context, params ListParams) (ListResult, error) {
	const op = "adminnotification.repo.ListByAdmin"

	if params.AdminID <= 0 {
		return ListResult{}, fmt.Errorf("%s: invalid admin id", op)
	}

	status := normalizeStatus(params.Status)

	builder := outbox.BuilderDollar().
		Select("id", "admin_id", "title", "message", "level", "payload", "is_read", "read_at", "created_at", "updated_at").
		From(tableName).
		Where(squirrel.Eq{"admin_id": params.AdminID}). //nolint:goconst // required
		OrderBy("created_at DESC", "id DESC").
		Limit(params.Limit).
		Offset(params.Offset)

	countBuilder := outbox.BuilderDollar().
		Select("count(*) AS total").
		From(tableName).
		Where(squirrel.Eq{"admin_id": params.AdminID})

	if status == StatusUnread {
		builder = builder.Where(squirrel.Eq{"is_read": false}) //nolint:goconst // required
		countBuilder = countBuilder.Where(squirrel.Eq{"is_read": false})
	}

	var items []models.AdminNotification
	if err := r.pgsql.DB().ScanAllx(ctx, op, &items, builder); err != nil {
		return ListResult{}, fmt.Errorf("%s: scan notifications: %w", op, outbox.ErrorTransform(err))
	}

	var total int
	if err := r.pgsql.DB().ScanOnex(ctx, op, &total, countBuilder); err != nil {
		return ListResult{}, fmt.Errorf("%s: count notifications: %w", op, outbox.ErrorTransform(err))
	}

	unread, err := r.CountUnread(ctx, params.AdminID)
	if err != nil {
		return ListResult{}, fmt.Errorf("%s: count unread: %w", op, err)
	}

	return ListResult{
		Notifications: items,
		Total:         total,
		UnreadCount:   unread,
	}, nil
}

func (r *Repo) CountUnread(ctx context.Context, adminID int64) (int, error) {
	const op = "adminnotification.repo.CountUnread"

	if adminID <= 0 {
		return 0, fmt.Errorf("%s: invalid admin id", op)
	}

	builder := outbox.BuilderDollar().
		Select("count(*) AS total").
		From(tableName).
		Where(squirrel.Eq{"admin_id": adminID}).
		Where(squirrel.Eq{"is_read": false})

	var total int
	if err := r.pgsql.DB().ScanOnex(ctx, op, &total, builder); err != nil {
		if pgxscan.NotFound(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("%s: count unread: %w", op, outbox.ErrorTransform(err))
	}

	return total, nil
}

func (r *Repo) MarkRead(ctx context.Context, adminID, notificationID int64) error {
	const op = "adminnotification.repo.MarkRead"

	if adminID <= 0 || notificationID <= 0 {
		return fmt.Errorf("%s: invalid ids", op)
	}

	now := time.Now()
	builder := outbox.BuilderDollar().
		Update(tableName).
		SetMap(querybuilder.Eq{
			"is_read":    true,
			"read_at":    now,
			"updated_at": now,
		}).
		Where(squirrel.Eq{"admin_id": adminID, "id": notificationID})

	tag, err := r.pgsql.DB().Execx(ctx, op, builder)
	if err != nil {
		return fmt.Errorf("%s: exec: %w", op, outbox.ErrorTransform(err))
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: notification not found", op)
	}

	return nil
}

func (r *Repo) MarkAllRead(ctx context.Context, adminID int64) (int64, error) {
	const op = "adminnotification.repo.MarkAllRead"

	if adminID <= 0 {
		return 0, fmt.Errorf("%s: invalid admin id", op)
	}

	now := time.Now()
	builder := outbox.BuilderDollar().
		Update(tableName).
		SetMap(querybuilder.Eq{
			"is_read":    true,
			"read_at":    now,
			"updated_at": now,
		}).
		Where(squirrel.Eq{"admin_id": adminID}).
		Where(squirrel.Eq{"is_read": false})

	tag, err := r.pgsql.DB().Execx(ctx, op, builder)
	if err != nil {
		return 0, fmt.Errorf("%s: exec: %w", op, outbox.ErrorTransform(err))
	}

	return tag.RowsAffected(), nil
}

func normalizeStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case StatusUnread:
		return StatusUnread
	default:
		return StatusAll
	}
}

func (r *Repo) Create(ctx context.Context, params CreateParams) (int64, error) {
	const op = "adminnotification.repo.Create"

	if params.AdminID <= 0 {
		return 0, fmt.Errorf("%s: invalid admin id", op)
	}
	if strings.TrimSpace(params.Title) == "" {
		return 0, fmt.Errorf("%s: empty title", op)
	}
	if strings.TrimSpace(params.Message) == "" {
		return 0, fmt.Errorf("%s: empty message", op)
	}
	if params.Payload == nil {
		params.Payload = models.NotificationPayload{}
	}

	builder := outbox.BuilderDollar().
		Insert(tableName).
		Columns("admin_id", "title", "message", "level", "payload").
		Values(params.AdminID, params.Title, params.Message, params.Level, params.Payload).
		Suffix("RETURNING id")
	if params.DispatchKey != "" {
		builder = outbox.BuilderDollar().Insert(tableName).
			Columns("admin_id", "title", "message", "level", "payload", "dispatch_key").
			Values(params.AdminID, params.Title, params.Message, params.Level, params.Payload, params.DispatchKey).
			Suffix("ON CONFLICT (admin_id, dispatch_key) DO UPDATE SET dispatch_key = EXCLUDED.dispatch_key RETURNING id")
	}

	var id int64
	if err := r.pgsql.DB().ScanOnex(ctx, op, &id, builder); err != nil {
		return 0, fmt.Errorf("%s: insert: %w", op, outbox.ErrorTransform(err))
	}

	return id, nil
}
