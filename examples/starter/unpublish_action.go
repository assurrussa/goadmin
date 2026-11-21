package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/netip"
	"net/url"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/assurrussa/goadmin/features/crud"
	"github.com/assurrussa/goadmin/features/mcp"
	"github.com/assurrussa/goadmin/host"
)

const (
	unpublishModuleKey = "product-unpublish-mcp"
	unpublishAgentPath = "/agent/product-unpublish"
)

type withdrawalInput struct {
	ProductID string `json:"product_id"`
	Reason    string `json:"reason"`
}

func normalizeWithdrawal(version string, in map[string]json.RawMessage) (json.RawMessage, error) {
	var input withdrawalInput
	if version != "1" || len(in) != 2 || json.Unmarshal(in["product_id"], &input.ProductID) != nil || json.Unmarshal(in["reason"], &input.Reason) != nil {
		return nil, crud.ErrInvalidQuery
	}
	id, err := strconv.ParseInt(input.ProductID, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != input.ProductID {
		return nil, crud.ErrInvalidQuery
	}
	input.Reason, err = unpublishReason.Normalize(input.Reason)
	if err != nil {
		return nil, crud.ErrInvalidQuery
	}
	return json.Marshal(input)
}

func productUnpublishModule(origin *url.URL, trustedProxy netip.Addr, showJournal bool) (host.Module, error) {
	action := mcp.Action{
		Namespace: "starter", Key: "product-unpublish", Version: "1", Path: unpublishAgentPath, Requires: []string{"crud.products"},
		Title: "Снятие товара с публикации", Description: "Клиент сможет предлагать снятие ваших товаров с публикации. После подтверждения товар перейдёт в черновик, а причина останется в журнале.",
		ConfirmLabel: "Снять с публикации", ProposeTool: "propose_unpublish", StatusTool: "get_unpublish_request",
		Permissions:  []host.PermissionKey{host.NewPermissionKey("products", "unpublish")},
		Parameters:   []mcp.Parameter{{Name: "product_id", Kind: mcp.PositiveID, Required: true, Description: "Published product ID."}, {Name: "reason", Kind: mcp.Text, Required: true, Description: "Required reason for withdrawal, up to 500 Unicode characters."}},
		ResultFields: []mcp.Field{{Key: "event_id", Kind: mcp.PositiveID, Description: "ID of the product journal entry."}},
		Normalize:    normalizeWithdrawal, Inspect: inspectWithdrawal, Apply: applyWithdrawal,
	}
	if showJournal {
		action.ReceiptLink = &mcp.ReceiptLink{
			Href: productEventsPath, Label: productEventsTitle,
			Permission: host.NewPermissionKey(productEventsPermission, "read"),
		}
	}
	return mcp.ConfirmedModule(mcp.Config{Origin: origin.String(), TrustedProxy: trustedProxy}, action)
}

func inspectWithdrawal(ctx context.Context, tx mcp.ActionTx, actor host.Actor, raw json.RawMessage, lock bool) (mcp.ActionSnapshot, error) {
	var input withdrawalInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return mcp.ActionSnapshot{}, err
	}
	query := `SELECT name,revision::text,status FROM starter_products WHERE owner_admin_id=$1 AND id=$2::text::bigint`
	if lock {
		query += " FOR UPDATE"
	}
	var name, revision, status string
	err := tx.QueryRow(ctx, query, actor.AdminID, input.ProductID).Scan(&name, &revision, &status)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && status != "published") {
		return mcp.ActionSnapshot{}, mcp.ErrActionNotFound
	}
	if err != nil {
		return mcp.ActionSnapshot{}, err
	}
	return mcp.ActionSnapshot{RecordID: input.ProductID, Revision: revision, Fields: []mcp.ReviewField{
		{Label: "Товар", Value: name}, {Label: "Переход", Value: "Опубликован → Черновик"}, {Label: "Причина", Value: input.Reason},
	}}, nil
}

// The single statement gives both entry points the same precondition, audit
// record and event ID. It participates in the caller's transaction.
const withdrawWithAudit = `WITH changed AS (` + unpublishProductUpdate + ` RETURNING id)
INSERT INTO starter_product_events(owner_admin_id,product_id,action,reason)
SELECT $1,changed.id::text,'unpublish',$4 FROM changed RETURNING id::text`

type (
	withdrawalRow   interface{ Scan(...any) error }
	withdrawalQuery func(context.Context, string, ...any) withdrawalRow
)

func withdrawProduct(ctx context.Context, query withdrawalQuery, actor host.Actor, id, revision, reason string) (string, error) {
	var event string
	err := query(ctx, withdrawWithAudit, actor.AdminID, id, revision, reason).Scan(&event)
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
		return "", crud.ErrConflict
	}
	return event, err
}

func applyWithdrawal(ctx context.Context, tx mcp.ActionTx, actor host.Actor, raw json.RawMessage, snapshot mcp.ActionSnapshot, _ string) (map[string]string, error) {
	var input withdrawalInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	event, err := withdrawProduct(ctx, func(ctx context.Context, sql string, args ...any) withdrawalRow {
		return tx.QueryRow(ctx, sql, args...)
	}, actor, snapshot.RecordID, snapshot.Revision, input.Reason)
	if err != nil {
		return nil, err
	}
	return map[string]string{"event_id": event}, nil
}
