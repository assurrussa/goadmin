//go:build integration

package main

import (
	"errors"
	"testing"

	"github.com/assurrussa/goadmin/features/crud"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

func TestProductEventsPostgresV2Upgrade(t *testing.T) {
	db, testDSN, owner := productTestDatabase(t, "product-events")
	ctx := t.Context()
	for _, statement := range []string{
		"ALTER TABLE starter_product_events DROP COLUMN reason",
		"DELETE FROM starter_product_migrations WHERE version=3",
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	var eventID string
	if err := db.QueryRowContext(ctx, "INSERT INTO starter_product_events(owner_admin_id,product_id,action) VALUES($1,'47','unpublish') RETURNING id::text", owner.AdminID).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := migrateProducts(ctx, testDSN); err != nil {
			t.Fatal(err)
		}
	}
	event, err := (productRepo{db}).event(ctx, owner, eventID)
	if err != nil || event.Data.ProductID != "47" || event.Data.Action != "unpublish" || event.Data.Reason != nil {
		t.Fatalf("upgraded historical event: %+v, %v", event, err)
	}
}

func TestProductEventsPostgresJournal(t *testing.T) {
	// The journal can be installed without mounting Products routes.
	db, _, owner := productTestDatabase(t, "product-events")
	ctx := t.Context()
	repo := productRepo{db}
	other := productTestActor(t, db, "00000000-0000-4000-8000-000000000002")
	// Numeric and text ordering of 9 and 10 differ when the timestamps tie.
	if _, err := db.ExecContext(ctx, "ALTER TABLE starter_product_events ALTER COLUMN id RESTART WITH 9"); err != nil {
		t.Fatal(err)
	}
	product, err := repo.create(ctx, owner, productInput{Name: "Archived"})
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.command(ctx, owner, product.ID, product.Revision, "deleted"); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.get(ctx, owner, product.ID); !errors.Is(err, crud.ErrNotFound) {
		t.Fatalf("deleted product lookup: %v", err)
	}
	second, err := repo.create(ctx, other, productInput{Name: "Other owner's product"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "INSERT INTO starter_product_events(owner_admin_id,product_id,action) VALUES($1,$2,'deleted')", owner.AdminID, product.ID+"0"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, "UPDATE starter_product_events SET created_at='2040-02-03 13:05:06+09'"); err != nil {
		t.Fatal(err)
	}

	query := datagrid.Filters{Search: product.ID, Page: 1, Limit: 1}
	for page, want := range []struct{ id, action string }{{"10", "deleted"}, {"9", "created"}} {
		query.Page = page + 1
		rows, total, listErr := repo.events(ctx, owner, query)
		if listErr != nil || total != 2 || len(rows) != 1 {
			t.Fatalf("page %d: rows=%v total=%d err=%v", query.Page, rows, total, listErr)
		}
		event := rows[0]
		if event.ID != want.id || event.Data.Action != want.action || event.Data.ProductID != product.ID || event.Data.CreatedAt != "2040-02-03 04:05:06 UTC" {
			t.Fatalf("page %d: event=%+v", query.Page, event)
		}
		detail, getErr := repo.event(ctx, owner, event.ID)
		if getErr != nil || detail != event || authorizeProductEvent(ctx, owner, detail) != nil {
			t.Fatalf("list/detail mismatch: %+v %v", detail, getErr)
		}
		if _, getErr = repo.event(ctx, other, event.ID); !errors.Is(getErr, crud.ErrNotFound) {
			t.Fatalf("foreign detail: %v", getErr)
		}
		if err = authorizeProductEvent(ctx, other, event); !errors.Is(err, crud.ErrDenied) {
			t.Fatalf("foreign row authorization: %v", err)
		}
	}
	query.SortOrder = "asc"
	for i, id := range []string{"9", "10"} {
		query.Page = i + 1
		rows, total, listErr := repo.events(ctx, owner, query)
		if listErr != nil || len(rows) != 1 || rows[0].ID != id || total != 2 {
			t.Fatalf("ascending page %d: rows=%v total=%d err=%v", query.Page, rows, total, listErr)
		}
	}
	query = datagrid.Filters{Page: 1, Limit: 100}
	otherEvents, total, err := repo.events(ctx, other, query)
	if err != nil || total != 1 || len(otherEvents) != 1 || otherEvents[0].Data.ProductID != second.ID {
		t.Fatalf("second owner's journal: %v, total=%d, err=%v", otherEvents, total, err)
	}
	query.Fields = map[string]any{"productId": product.ID}
	rows, total, err := repo.events(ctx, other, query)
	if err != nil || len(rows) != 0 || total != 0 {
		t.Fatalf("foreign product filter: %v, total=%d, err=%v", rows, total, err)
	}
	filtered, total, err := repo.events(ctx, owner, query)
	if err != nil || total != 2 || len(filtered) != 2 {
		t.Fatalf("archived product filter: %v, total=%d, err=%v", filtered, total, err)
	}
	query.Fields["productId"] = 123
	rows, total, err = repo.events(ctx, owner, query)
	var fields crud.FieldErrors
	if !errors.As(err, &fields) || len(fields["productId"]) == 0 || len(rows) != 0 || total != 0 {
		t.Fatalf("invalid product filter: %v, total=%d, err=%v", rows, total, err)
	}
}

type journalSnapshotFilter struct {
	datagrid.Filters
	beforePage func()
}

func (f journalSnapshotFilter) GetOffset() int {
	f.beforePage()
	return f.Filters.GetOffset()
}

func TestProductEventsPostgresSearchSnapshot(t *testing.T) {
	db, _, owner := productTestDatabase(t, "product-events")
	ctx := t.Context()
	repo := productRepo{db}
	filter := datagrid.Filters{Page: 1, Limit: 25, Fields: map[string]any{"productId": "47", "action": "unpublish"}}

	// Commit the first event on another connection after count(*) has established
	// its snapshot and before the page query starts.
	var firstID string
	rows, total, err := repo.events(ctx, owner, journalSnapshotFilter{filter, func() {
		err := db.QueryRowContext(ctx, "INSERT INTO starter_product_events(owner_admin_id,product_id,action,reason) VALUES($1,'47','unpublish','Archived') RETURNING id::text", owner.AdminID).Scan(&firstID)
		if err != nil {
			t.Fatal(err)
		}
	}})
	if err != nil || total != 0 || len(rows) != 0 {
		t.Fatalf("inconsistent initial snapshot: rows=%v total=%d err=%v", rows, total, err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO starter_product_events(owner_admin_id,product_id,action) SELECT $1,'47','updated' FROM generate_series(1,101)", owner.AdminID); err != nil {
		t.Fatal(err)
	}
	rows, total, err = repo.events(ctx, owner, filter)
	if err != nil || total != 1 || len(rows) != 1 || rows[0].ID != firstID || rows[0].Data.Reason != nil {
		t.Fatalf("historical unpublish filter: rows=%v total=%d err=%v", rows, total, err)
	}
	filter.Page = 2
	rows, total, err = repo.events(ctx, owner, filter)
	if err != nil || total != 1 || len(rows) != 0 {
		t.Fatalf("past last page: rows=%v total=%d err=%v", rows, total, err)
	}
}
