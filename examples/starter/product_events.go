package main

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/assurrussa/goadmin/features/crud"
	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

type productEvent struct {
	ProductID string  `json:"productId"`
	Action    string  `json:"action"`
	CreatedAt string  `json:"createdAt"`
	Reason    *string `json:"reason,omitempty"`
	ownerID   int64
}

const eventColumns = "id::text, owner_admin_id, product_id, action, created_at"

func scanProductEvent(scanner interface{ Scan(...any) error }) (crud.ReadOnlyRecord[productEvent], error) {
	var event crud.ReadOnlyRecord[productEvent]
	var created time.Time
	err := scanner.Scan(&event.ID, &event.Data.ownerID, &event.Data.ProductID, &event.Data.Action, &created, &event.Data.Reason)
	if errors.Is(err, sql.ErrNoRows) {
		return event, crud.ErrNotFound
	}
	if err == nil {
		event.Data.CreatedAt = created.UTC().Format(time.DateTime) + " UTC"
	}
	return event, err
}

func (r productRepo) event(ctx context.Context, actor host.Actor, id string) (crud.ReadOnlyRecord[productEvent], error) {
	return scanProductEvent(r.db.QueryRowContext(ctx, "SELECT "+eventColumns+", reason FROM starter_product_events WHERE owner_admin_id=$1 AND id::text=$2", actor.AdminID, id))
}

func (r productRepo) events(ctx context.Context, actor host.Actor, f datagrid.Filtered) ([]crud.ReadOnlyRecord[productEvent], int, error) {
	// Read the event owner directly so deleting a product keeps its history accessible.
	const from = ` FROM starter_product_events AS e WHERE owner_admin_id=$1
		AND ($2::text='' OR product_id=$2) AND ($3::text='' OR product_id=$3)`
	var productID string
	if value, present := f.GetFields()["productId"]; present {
		var ok bool
		if productID, ok = value.(string); !ok {
			return nil, 0, crud.FieldErrors{"productId": {"Product number must be text"}}
		}
	}
	search := f.GetSearch()
	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT count(*)"+from, actor.AdminID, search, productID).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := " ORDER BY e.created_at DESC, e.id DESC"
	if f.GetSortOrder() == "asc" {
		order = " ORDER BY e.created_at ASC, e.id ASC"
	}
	// Only the owner-scoped detail query fetches the operator's explanation.
	rows, err := r.db.QueryContext(ctx, "SELECT "+eventColumns+", NULL::text"+from+order+" LIMIT $4 OFFSET $5", actor.AdminID, search, productID, f.GetLimit(), f.GetOffset())
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	events := []crud.ReadOnlyRecord[productEvent]{}
	for rows.Next() {
		event, scanErr := scanProductEvent(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		events = append(events, event)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, err
	}
	return events, total, nil
}

func authorizeProductEvent(_ context.Context, actor host.Actor, event crud.ReadOnlyRecord[productEvent]) error {
	if actor.AdminID <= 0 || event.Data.ownerID != actor.AdminID {
		return crud.ErrDenied
	}
	return nil
}

func productEventsModule(db *sql.DB) (host.Module, error) {
	repo := productRepo{db}
	permission := host.NewPermissionKey("product_events", "read")
	const title = "Журнал действий с товарами"
	actions := []crud.Option{
		{Value: "created", Label: "Создан"},
		{Value: "updated", Label: "Обновлён"},
		{Value: "publish", Label: "Опубликован"},
		{Value: "unpublish", Label: "Снят с публикации"},
		{Value: "deleted", Label: "Удалён"},
	}
	badges := make(map[string]map[string]string, len(actions))
	for _, action := range actions {
		badges[action.Value] = map[string]string{"label": action.Label, "variant": "secondary"}
	}
	return crud.NewReadOnly(crud.ReadOnlyDefinition[productEvent]{
		Key: "product-events", BasePath: "/product-events", Title: title, Permission: permission,
		Menu: host.Menu{Sections: []host.Section{{Key: "products", Title: "Products", Items: []host.Item{
			{Name: title, Href: "/product-events", Icon: "content", PermissionKey: permission},
		}}}},
		Fields: []crud.Field{
			{Key: "productId", Label: "Номер товара", Type: "text", Read: true},
			{Key: "action", Label: "Действие", Type: "select", Options: actions, Read: true},
			{Key: "createdAt", Label: "Время (UTC)", Type: "text", Read: true},
			{Key: unpublishReason.Key, Label: unpublishReason.Label, Type: unpublishReason.Type, Read: true},
		},
		Columns: []datagrid.Column{
			{Key: "productId", Label: "Номер товара", Type: "text", Filterable: true},
			{Key: "action", Label: "Действие", Type: "select", Badges: badges},
			{Key: "createdAt", Label: "Время (UTC)", Type: "text", Sortable: true},
		},
		DefaultSort: "createdAt", DefaultOrder: "desc",
		List: repo.events, Get: repo.event, Authorize: authorizeProductEvent,
	})
}
