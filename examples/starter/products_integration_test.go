//go:build integration

package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/assurrussa/goadmin/features/crud"
	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

func productTestDatabase(t *testing.T, module string) (*sql.DB, string, host.Actor) {
	t.Helper()
	dsn := os.Getenv("PRODUCTS_TEST_DSN")
	if dsn == "" {
		t.Skip("PRODUCTS_TEST_DSN must name an isolated PostgreSQL test server")
	}
	ctx := context.Background()
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	database := fmt.Sprintf("products_test_%d", time.Now().UnixNano())
	if _, err = admin.ExecContext(ctx, "CREATE DATABASE "+database); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, e := admin.ExecContext(ctx, "DROP DATABASE "+database+" WITH (FORCE)"); e != nil {
			t.Error(e)
		}
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + database
	testDSN := u.String()
	// Explicit migrate must install core and selected Products even without the
	// automatic-startup flag; it must return without opening the HTTP listener.
	t.Setenv("DATABASE_DSN", testDSN)
	t.Setenv("ADMIN_MODULES", module)
	t.Setenv("ADMIN_URL", "http://localhost:18768")
	t.Setenv("CSRF_SECRET", strings.Repeat("c", 32))
	t.Setenv("FIRST_ADMIN_SETUP_TOKEN", strings.Repeat("s", 40))
	t.Setenv("STARTER_MIGRATE", "0")
	for index, key := range []string{"GOAUTH_SIGNING_KEY", "GOAUTH_TOKEN_KEY", "GOAUTH_ENVELOPE_KEY"} {
		t.Setenv(key, base64.StdEncoding.EncodeToString([]byte(strings.Repeat(string(rune('a'+index)), 32))))
	}
	oldArgs := os.Args
	os.Args = []string{"starter", "migrate"}
	t.Cleanup(func() { os.Args = oldArgs })
	if err = run(); err != nil {
		t.Fatal(err)
	}
	os.Args = oldArgs
	db, err := sql.Open("pgx", testDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, testDSN, productTestActor(t, db, "00000000-0000-4000-8000-000000000001")
}

func productTestActor(t *testing.T, db *sql.DB, subjectID string) host.Actor {
	t.Helper()
	actor := host.Actor{SubjectID: subjectID}
	if _, err := db.ExecContext(t.Context(), "INSERT INTO auth_subjects(id,status,created_at,updated_at) VALUES($1,'active',NOW(),NOW())", actor.SubjectID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), "INSERT INTO administrations(subject_id,uuid) VALUES($1,gen_random_uuid()) RETURNING id", actor.SubjectID).Scan(&actor.AdminID); err != nil {
		t.Fatal(err)
	}
	return actor
}

func TestProductsPostgresConstraintsAndLegacyCorrection(t *testing.T) {
	db, _, actor := productTestDatabase(t, "products")
	repo := productRepo{db}
	ctx := t.Context()
	validName := strings.Repeat("🙂", 200)
	invalid := productInput{Name: validName + "🙂", Stock: -1}

	// Represent a host database that already contains a product outside today's rules.
	var id string
	if err := db.QueryRowContext(ctx, "INSERT INTO starter_products(owner_admin_id,name,description,stock,active,available,status) VALUES($1,$2,'',$3,false,'','draft') RETURNING id::text", actor.AdminID, invalid.Name, invalid.Stock).Scan(&id); err != nil {
		t.Fatal(err)
	}
	legacy, err := repo.get(ctx, actor, id)
	if err != nil || legacy.Data.input() != invalid {
		t.Fatalf("read legacy product: %+v, %v", legacy, err)
	}
	rows, total, err := repo.list(ctx, actor, datagrid.Filters{Page: 1, Limit: 20})
	if err != nil || total != 1 || len(rows) != 1 || rows[0] != legacy {
		t.Fatalf("list legacy product: %+v, total %d, %v", rows, total, err)
	}

	_, createErr := repo.create(ctx, actor, invalid)
	_, updateErr := repo.update(ctx, actor, id, legacy.Revision, invalid)
	for _, writeErr := range []error{createErr, updateErr} {
		var fields crud.FieldErrors
		if !errors.As(writeErr, &fields) || len(fields["name"]) != 1 || len(fields["stock"]) != 1 {
			t.Fatalf("expected both field errors, got %#v", writeErr)
		}
	}
	unchanged, total, err := repo.list(ctx, actor, datagrid.Filters{Page: 1, Limit: 20})
	if err != nil || total != 1 || len(unchanged) != 1 || unchanged[0] != legacy {
		t.Fatalf("rejected writes changed products: %+v, total %d, %v", unchanged, total, err)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM starter_product_events").Scan(&count); err != nil || count != 0 {
		t.Fatalf("rejected writes created events: %d, %v", count, err)
	}
	corrected := productInput{Name: validName, Stock: 0}
	saved, err := repo.update(ctx, actor, id, legacy.Revision, corrected)
	if err != nil || saved.Revision == legacy.Revision || saved.Data.input() != corrected {
		t.Fatalf("correct legacy product: %+v, %v", saved, err)
	}
}

func TestProductsPostgresCASIsolationAndAuditRollback(t *testing.T) {
	db, testDSN, actor := productTestDatabase(t, "products")
	other := productTestActor(t, db, "00000000-0000-4000-8000-000000000002")
	ctx := t.Context()
	repo := productRepo{db}
	for range 2 {
		if err := migrateProducts(ctx, testDSN); err != nil {
			t.Fatal(err)
		}
	}
	// Upgrade an existing version-1 installation without resetting its rows/audit.
	for _, statement := range []string{
		"ALTER TABLE starter_products DROP COLUMN category",
		"ALTER TABLE starter_product_events DROP COLUMN reason",
		"DELETE FROM starter_product_migrations WHERE version IN (2,3)",
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	var legacyID string
	if err := db.QueryRowContext(ctx, "INSERT INTO starter_products(owner_admin_id,revision,name,description,stock,active,available,status) VALUES($1,7,'Legacy','retained',4,true,'','draft') RETURNING id::text", actor.AdminID).Scan(&legacyID); err != nil {
		t.Fatal(err)
	}
	if err := migrateProducts(ctx, testDSN); err != nil {
		t.Fatal(err)
	}
	legacy, err := repo.get(ctx, actor, legacyID)
	if err != nil || legacy.Revision != "7" || legacy.Data.Category != "" || legacy.Data.Name != "Legacy" {
		t.Fatalf("legacy migration: %+v %v", legacy, err)
	}
	data := productInput{Name: "Initial", Stock: 3}
	original, err := repo.create(ctx, actor, data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.get(ctx, other, original.ID); !errors.Is(err, crud.ErrNotFound) {
		t.Fatalf("cross-owner get: %v", err)
	}
	rows, total, err := repo.list(ctx, other, datagrid.Filters{Page: 1, Limit: 20})
	if err != nil || len(rows) != 0 || total != 0 {
		t.Fatalf("scope: %v %d %d", err, len(rows), total)
	}
	start := make(chan struct{})
	outcomes := make(chan error, 2)
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Go(func() {
			<-start
			p := data
			p.Name = fmt.Sprintf("Winner %d", i)
			_, e := repo.update(ctx, actor, original.ID, original.Revision, p)
			outcomes <- e
		})
	}
	close(start)
	wg.Wait()
	close(outcomes)
	successes, conflicts := 0, 0
	for e := range outcomes {
		if e == nil {
			successes++
		} else if errors.Is(e, crud.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("CAS: %d successes %d conflicts", successes, conflicts)
	}
	current, err := repo.get(ctx, actor, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRowContext(ctx, "SELECT count(*) FROM starter_product_events WHERE action='updated'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("audit winners %d %v", count, err)
	}
	if _, err = db.ExecContext(ctx, "ALTER TABLE starter_product_events ADD CONSTRAINT test_audit_failure CHECK(action <> 'updated') NOT VALID"); err != nil {
		t.Fatal(err)
	}
	changed := current.Data.input()
	changed.Name = "Must roll back"
	if _, err = repo.update(ctx, actor, current.ID, current.Revision, changed); err == nil {
		t.Fatal("audit INSERT unexpectedly succeeded")
	}
	after, err := repo.get(ctx, actor, current.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after != current {
		t.Fatalf("write escaped audit rollback: %+v -> %+v", current, after)
	}
	if _, err = db.ExecContext(ctx, "ALTER TABLE starter_product_events DROP CONSTRAINT test_audit_failure"); err != nil {
		t.Fatal(err)
	}
	if err = repo.command(ctx, actor, current.ID, original.Revision, "deleted"); !errors.Is(err, crud.ErrConflict) {
		t.Fatalf("stale delete %v", err)
	}
	raceProductCommand(t, "publish", func(_ int) error {
		return repo.command(ctx, actor, current.ID, current.Revision, "publish")
	})
	published, err := repo.get(ctx, actor, current.ID)
	if err != nil || published.Data.Status != "published" {
		t.Fatalf("publish %v %+v", err, published)
	}
	editedPublished, err := repo.update(ctx, actor, published.ID, published.Revision, productInput{Name: "Published edit", Stock: 2, Category: "premium"})
	if err != nil || editedPublished.Data.Status != "published" {
		t.Fatalf("ordinary update changed publication: %+v %v", editedPublished, err)
	}
	published = editedPublished
	for _, action := range []string{"unknown", "unpublish"} {
		if err = repo.command(ctx, actor, published.ID, published.Revision, action); !errors.Is(err, crud.ErrDenied) {
			t.Fatalf("parameterless %s must be denied: %v", action, err)
		}
	}
	input := UnpublishInput{Reason: "Rejected attempt"}
	for _, owner := range []host.Actor{actor, other} {
		revision := published.Revision
		if owner.AdminID == actor.AdminID {
			revision = original.Revision
		}
		if err = repo.unpublish(ctx, owner, published.ID, revision, input); !errors.Is(err, crud.ErrConflict) {
			t.Fatalf("stale/cross-owner unpublish: %v", err)
		}
	}
	if _, err = db.ExecContext(ctx, "ALTER TABLE starter_product_events ADD CONSTRAINT test_unpublish_audit_failure CHECK(action <> 'unpublish') NOT VALID"); err != nil {
		t.Fatal(err)
	}
	if err = repo.unpublish(ctx, actor, published.ID, published.Revision, input); err == nil {
		t.Fatal("unpublish audit failure unexpectedly succeeded")
	}
	after, err = repo.get(ctx, actor, published.ID)
	if err != nil || after != published {
		t.Fatalf("unpublish escaped audit rollback: %+v %v", after, err)
	}
	if _, err = db.ExecContext(ctx, "ALTER TABLE starter_product_events DROP CONSTRAINT test_unpublish_audit_failure"); err != nil {
		t.Fatal(err)
	}
	reasons := [2]string{"Missing details\n  in description", "\uFEFF" + strings.Repeat("🙂", 499)}
	winner := raceProductCommand(t, "unpublish", func(i int) error {
		return repo.unpublish(ctx, actor, published.ID, published.Revision, UnpublishInput{Reason: "\u0085 " + reasons[i] + "\n\t"})
	})
	revision, err := strconv.ParseInt(published.Revision, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := repo.get(ctx, actor, published.ID)
	if err != nil || draft.Data.Status != "draft" || draft.Revision != strconv.FormatInt(revision+1, 10) {
		t.Fatalf("unpublish: %+v %v", draft, err)
	}
	if err = repo.unpublish(ctx, actor, draft.ID, draft.Revision, input); !errors.Is(err, crud.ErrConflict) {
		t.Fatalf("cannot unpublish a draft: %v", err)
	}
	if err = db.QueryRowContext(ctx, "SELECT count(*) FROM starter_product_events WHERE product_id=$1 AND action='unpublish'", draft.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("unpublish audit count %d %v", count, err)
	}
	if err = repo.command(ctx, actor, draft.ID, draft.Revision, "deleted"); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.get(ctx, actor, draft.ID); !errors.Is(err, crud.ErrNotFound) {
		t.Fatalf("delete %v", err)
	}
	events, _, err := repo.events(ctx, actor, datagrid.Filters{Page: 1, Limit: 100, Search: draft.ID})
	if err != nil {
		t.Fatal(err)
	}
	var unpublishID string
	for _, row := range events {
		if row.Data.Reason != nil {
			t.Fatal("event list exposed a reason")
		}
		if row.Data.Action == "unpublish" {
			unpublishID = row.ID
		}
	}
	detail, err := repo.event(ctx, actor, unpublishID)
	if err != nil || detail.Data.Reason == nil || *detail.Data.Reason != reasons[winner] {
		t.Fatalf("archived unpublish reason did not match the winning command: %+v, %v", detail, err)
	}
	if _, err = repo.event(ctx, other, unpublishID); !errors.Is(err, crud.ErrNotFound) {
		t.Fatalf("foreign event lookup: %v", err)
	}
}

func raceProductCommand(t *testing.T, action string, command func(int) error) int {
	t.Helper()
	start := make(chan struct{})
	var results [2]error
	var workers sync.WaitGroup
	for i := range results {
		workers.Go(func() {
			<-start
			results[i] = command(i)
		})
	}
	close(start)
	workers.Wait()
	for winner, err := range results {
		if err == nil && errors.Is(results[1-winner], crud.ErrConflict) {
			return winner
		}
	}
	t.Fatalf("%s CAS: expected one winner and one conflict, got %v", action, results)
	return -1
}
