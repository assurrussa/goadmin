package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/assurrussa/goadmin/features/crud"
	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

type requestData struct {
	Subject string  `json:"subject"`
	Status  string  `json:"status"`
	Reason  *string `json:"reason"`
}

type rejectRequestInput struct {
	Reason string `json:"reason"`
}

var rejectionReason = crud.ActionParam{
	Key: "reason", Label: "Причина", Type: "textarea", Required: true, TrimSpace: true, MaxLength: new(500),
}

type requestRepo struct{ db *sql.DB }

func requestsModule(db *sql.DB) (host.Module, error) {
	repo := requestRepo{db}
	permission := func(action host.PermissionAction) host.PermissionKey {
		return host.NewPermissionKey("requests", action)
	}
	return crud.NewWithActions(crud.WithActionsDefinition[requestData]{
		Key: "requests", BasePath: "/requests", Title: "Заявки", Permission: permission("read"),
		Menu: host.Menu{Sections: []host.Section{{Key: "requests", Title: "Заявки", Items: []host.Item{
			{Name: "Заявки", Href: "/requests", Icon: "content", PermissionKey: permission("read")},
		}}}},
		Fields: []crud.Field{
			{Key: "subject", Label: "Тема", Type: "text", Read: true},
			{Key: "status", Label: "Статус", Type: "select", Read: true, Options: []crud.Option{
				{Value: "pending", Label: "Ожидает решения"}, {Value: "approved", Label: "Одобрена"}, {Value: "rejected", Label: "Отклонена"},
			}},
			{Key: "reason", Label: "Причина отклонения", Type: "textarea", Read: true},
		},
		ListColumns: []crud.ListColumn{{Key: "subject", Sortable: true}, {Key: "status", Filterable: true}},
		List:        repo.list, Get: repo.get, Authorize: authorizeRequest,
		Actions: []crud.Action[requestData]{
			{Key: "approve", Label: "Одобрить", Confirmation: "Одобрить эту заявку?", Permission: permission("approve"), Placement: crud.Detail, Run: repo.approve},
			crud.ActionWithParams(crud.Action[requestData]{Key: "reject", Label: "Отклонить", Confirmation: "Отклонить эту заявку?", Permission: permission("reject"), Placement: crud.Detail},
				[]crud.ActionParam{rejectionReason}, repo.reject),
		},
	})
}

func authorizeRequest(_ context.Context, actor host.Actor, op crud.Operation, row crud.Record[requestData]) error {
	if actor.AdminID > 0 {
		if op == crud.Read || ((op == "approve" || op == "reject") && row.Data.Status == "pending") {
			return nil
		}
	}
	return crud.ErrDenied
}

// The queue is shared by every operator with requests:read. Only details load the decision reason.
func (r requestRepo) get(ctx context.Context, _ host.Actor, id string) (crud.Record[requestData], error) {
	requestID, ok := parseRequestID(id)
	if !ok {
		return crud.Record[requestData]{}, crud.ErrNotFound
	}
	return scanRequest(r.db.QueryRowContext(ctx, `SELECT r.id::text,r.revision::text,r.subject,r.status,d.reason
		FROM starter_requests r LEFT JOIN starter_request_decisions d ON d.request_id=r.id WHERE r.id=$1`, requestID))
}

func parseRequestID(id string) (int64, bool) {
	value, err := strconv.ParseInt(id, 10, 64)
	return value, err == nil && value > 0 && strconv.FormatInt(value, 10) == id
}

func scanRequest(scanner interface{ Scan(...any) error }) (crud.Record[requestData], error) {
	var row crud.Record[requestData]
	err := scanner.Scan(&row.ID, &row.Revision, &row.Data.Subject, &row.Data.Status, &row.Data.Reason)
	if errors.Is(err, sql.ErrNoRows) {
		err = crud.ErrNotFound
	}
	return row, err
}

func (r requestRepo) list(ctx context.Context, _ host.Actor, f datagrid.Filtered) ([]crud.Record[requestData], int, error) {
	const from = ` FROM starter_requests WHERE ($1::text='' OR subject ILIKE '%' || $1 || '%') AND ($2::text='' OR status=$2)`
	status, _ := f.GetFields()["status"].(string)
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	var total int
	if err := tx.QueryRowContext(ctx, "SELECT count(*)"+from, f.GetSearch(), status).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := " ORDER BY subject ASC,id ASC"
	if f.GetSortOrder() == "desc" {
		order = " ORDER BY subject DESC,id DESC"
	}
	rows, err := tx.QueryContext(ctx, "SELECT id::text,revision::text,subject,status,NULL::text"+from+order+" LIMIT $3 OFFSET $4", f.GetSearch(), status, f.GetLimit(), f.GetOffset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	records := []crud.Record[requestData]{}
	for rows.Next() {
		row, err := scanRequest(rows)
		if err != nil {
			return nil, 0, err
		}
		records = append(records, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return records, total, tx.Commit()
}

func (r requestRepo) approve(ctx context.Context, actor host.Actor, id, revision string) error {
	return r.decide(ctx, actor, id, revision, "approved", nil)
}

func (r requestRepo) reject(ctx context.Context, actor host.Actor, id, revision string, input rejectRequestInput) error {
	reason, err := rejectionReason.Normalize(input.Reason)
	if err != nil {
		return crud.FieldErrors{"reason": {err.Error()}}
	}
	return r.decide(ctx, actor, id, revision, "rejected", &reason)
}

func (r requestRepo) decide(ctx context.Context, actor host.Actor, id, revision, status string, reason *string) error {
	requestID, ok := parseRequestID(id)
	if !ok {
		return crud.ErrConflict
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, `UPDATE starter_requests SET status=$3,revision=revision+1
		WHERE id=$1 AND revision::text=$2 AND status='pending' RETURNING id`, requestID, revision, status).Scan(&requestID)
	if errors.Is(err, sql.ErrNoRows) {
		return crud.ErrConflict
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO starter_request_decisions(request_id,status,admin_id,reason)
		VALUES ($1,$2,$3,$4)`, requestID, status, actor.AdminID, reason); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%w: %v", crud.ErrUnknown, err)
	}
	return nil
}
