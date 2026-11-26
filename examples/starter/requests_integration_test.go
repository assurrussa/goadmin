//go:build integration

package main

import (
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/assurrussa/goadmin/features/crud"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

func TestRequestsLateEnableAndConcurrentMigration(t *testing.T) {
	db, dsn, _ := productTestDatabase(t, "")
	ctx := t.Context()
	var absent bool
	if err := db.QueryRowContext(ctx, "SELECT to_regclass('starter_request_migrations') IS NULL").Scan(&absent); err != nil || !absent {
		t.Fatalf("disabled requests installed storage: %v", err)
	}
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			if err := migrateRequests(ctx, dsn); err != nil {
				t.Errorf("concurrent request migration: %v", err)
			}
		})
	}
	wg.Wait()
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM starter_requests").Scan(&count); err != nil || count != 6 {
		t.Fatalf("seeds after concurrent migration: %d, %v", count, err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM starter_requests"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ADMIN_MODULES", "requests")
	oldArgs := os.Args
	os.Args = []string{"starter", "migrate"}
	defer func() { os.Args = oldArgs }()
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM starter_requests").Scan(&count); err != nil || count != 0 {
		t.Fatalf("re-enabled requests resurrected demo rows: %d, %v", count, err)
	}
}

func TestRequestsConcurrentDecision(t *testing.T) {
	db, _, first := productTestDatabase(t, "requests")
	second := productTestActor(t, db, "00000000-0000-4000-8000-000000000002")
	ctx := t.Context()
	repo := requestRepo{db}
	rows, total, err := repo.list(ctx, first, datagrid.Filters{Page: 2, Limit: 2, Search: "Доступ", SortOrder: "desc", Fields: map[string]any{"status": "pending"}})
	if err != nil || len(rows) != 2 || total != 6 {
		t.Fatalf("shared queue: %v records, total %d, %v", len(rows), total, err)
	}
	row, err := repo.get(ctx, second, rows[0].ID)
	if err != nil || row != rows[0] {
		t.Fatalf("second operator read: %+v, %v", row, err)
	}
	for _, id := range []string{"0" + row.ID, "+" + row.ID, "-1", "0", "9223372036854775808", "invalid"} {
		if _, err := repo.get(ctx, first, id); !errors.Is(err, crud.ErrNotFound) {
			t.Fatalf("read with invalid ID %q: %v", id, err)
		}
		if err := repo.approve(ctx, first, id, row.Revision); !errors.Is(err, crud.ErrConflict) {
			t.Fatalf("decision with invalid ID %q: %v", id, err)
		}
	}
	var invalid crud.FieldErrors
	if err := repo.reject(ctx, second, row.ID, row.Revision, rejectRequestInput{"  "}); !errors.As(err, &invalid) {
		t.Fatalf("empty reason: %v", err)
	}
	start := make(chan struct{})
	decision := make(chan error, 2)
	go func() {
		<-start
		decision <- repo.approve(ctx, first, row.ID, row.Revision)
	}()
	go func() {
		<-start
		decision <- repo.reject(ctx, second, row.ID, row.Revision, rejectRequestInput{"  Проверено  "})
	}()
	close(start)
	a, b := <-decision, <-decision
	if !(a == nil && errors.Is(b, crud.ErrConflict) || errors.Is(a, crud.ErrConflict) && b == nil) {
		t.Fatalf("concurrent decisions: %v, %v", a, b)
	}
	got, err := repo.get(ctx, first, row.ID)
	if err != nil || got.Revision == row.Revision || got.Data.Status == "pending" {
		t.Fatalf("decided request: %+v, %v", got, err)
	}
	var audit struct {
		status string
		actor  int64
		count  int
	}
	if err := db.QueryRowContext(ctx, `SELECT status,admin_id,count(*) OVER () FROM starter_request_decisions WHERE request_id::text=$1`, row.ID).
		Scan(&audit.status, &audit.actor, &audit.count); err != nil {
		t.Fatal(err)
	}
	if audit.count != 1 || audit.status != got.Data.Status {
		t.Fatalf("audit decision: %+v, request %+v", audit, got.Data)
	}
	if got.Data.Status == "approved" && (audit.actor != first.AdminID || got.Data.Reason != nil) ||
		got.Data.Status == "rejected" && (audit.actor != second.AdminID || got.Data.Reason == nil || *got.Data.Reason != "Проверено") {
		t.Fatalf("audit actor/reason: %+v, request %+v", audit, got.Data)
	}
}
