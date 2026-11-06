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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/assurrussa/goadmin/features/crud"
	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

func TestProductsPostgresCASIsolationAndAuditRollback(t *testing.T) {
	dsn := os.Getenv("PRODUCTS_TEST_DSN")
	if dsn == "" {
		t.Skip("PRODUCTS_TEST_DSN must name an isolated PostgreSQL test server")
	}
	ctx := context.Background()
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	database := fmt.Sprintf("products_test_%d", time.Now().UnixNano())
	if _, err = admin.ExecContext(ctx, "CREATE DATABASE "+database); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, e := admin.ExecContext(ctx, "DROP DATABASE "+database+" WITH (FORCE)"); e != nil {
			t.Error(e)
		}
	}()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + database
	testDSN := u.String()
	// Explicit migrate must install core and selected Products even without the
	// automatic-startup flag; it must return without opening the HTTP listener.
	t.Setenv("DATABASE_DSN", testDSN)
	t.Setenv("ADMIN_MODULES", "products")
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
	if err = migrateProducts(ctx, testDSN); err != nil {
		t.Fatal(err)
	}
	if err = migrateProducts(ctx, testDSN); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", testDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	actor := host.Actor{SubjectID: "00000000-0000-4000-8000-000000000001"}
	if _, err = db.ExecContext(ctx, "INSERT INTO auth_subjects(id,status,created_at,updated_at) VALUES($1,'active',NOW(),NOW())", actor.SubjectID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, "INSERT INTO administrations(subject_id,uuid) VALUES($1,gen_random_uuid()) RETURNING id", actor.SubjectID).Scan(&actor.AdminID); err != nil {
		t.Fatal(err)
	}
	repo := productRepo{db}
	// Upgrade an existing version-1 installation without resetting its rows/audit.
	for _, statement := range []string{
		"ALTER TABLE starter_products DROP COLUMN category",
		"DELETE FROM starter_product_migrations WHERE version=2",
	} {
		if _, err = db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	var legacyID string
	if err = db.QueryRowContext(ctx, "INSERT INTO starter_products(owner_admin_id,revision,name,description,stock,active,available,status) VALUES($1,7,'Legacy','retained',4,true,'','draft') RETURNING id::text", actor.AdminID).Scan(&legacyID); err != nil {
		t.Fatal(err)
	}
	if err = migrateProducts(ctx, testDSN); err != nil {
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
	if _, err = repo.get(ctx, host.Actor{AdminID: actor.AdminID + 1}, original.ID); !errors.Is(err, crud.ErrNotFound) {
		t.Fatalf("cross-owner get: %v", err)
	}
	rows, total, err := repo.list(ctx, host.Actor{AdminID: actor.AdminID + 1}, datagrid.Filters{Page: 1, Limit: 20})
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
	raceProductCommand(t, repo, actor, current.ID, current.Revision, "publish")
	published, err := repo.get(ctx, actor, current.ID)
	if err != nil || published.Data.Status != "published" {
		t.Fatalf("publish %v %+v", err, published)
	}
	editedPublished, err := repo.update(ctx, actor, published.ID, published.Revision, productInput{Name: "Published edit", Stock: 2, Category: "premium"})
	if err != nil || editedPublished.Data.Status != "published" {
		t.Fatalf("ordinary update changed publication: %+v %v", editedPublished, err)
	}
	published = editedPublished
	if err = repo.command(ctx, actor, published.ID, published.Revision, "unknown"); !errors.Is(err, crud.ErrDenied) {
		t.Fatalf("unknown command must be denied: %v", err)
	}
	for _, owner := range []host.Actor{actor, {AdminID: actor.AdminID + 1}} {
		revision := published.Revision
		if owner.AdminID == actor.AdminID {
			revision = original.Revision
		}
		if err = repo.command(ctx, owner, published.ID, revision, "unpublish"); !errors.Is(err, crud.ErrConflict) {
			t.Fatalf("stale/cross-owner unpublish: %v", err)
		}
	}
	if _, err = db.ExecContext(ctx, "ALTER TABLE starter_product_events ADD CONSTRAINT test_unpublish_audit_failure CHECK(action <> 'unpublish') NOT VALID"); err != nil {
		t.Fatal(err)
	}
	if err = repo.command(ctx, actor, published.ID, published.Revision, "unpublish"); err == nil {
		t.Fatal("unpublish audit failure unexpectedly succeeded")
	}
	after, err = repo.get(ctx, actor, published.ID)
	if err != nil || after != published {
		t.Fatalf("unpublish escaped audit rollback: %+v %v", after, err)
	}
	if _, err = db.ExecContext(ctx, "ALTER TABLE starter_product_events DROP CONSTRAINT test_unpublish_audit_failure"); err != nil {
		t.Fatal(err)
	}
	raceProductCommand(t, repo, actor, published.ID, published.Revision, "unpublish")
	draft, err := repo.get(ctx, actor, published.ID)
	if err != nil || draft.Data.Status != "draft" || draft.Revision == published.Revision {
		t.Fatalf("unpublish: %+v %v", draft, err)
	}
	if err = repo.command(ctx, actor, draft.ID, draft.Revision, "unpublish"); !errors.Is(err, crud.ErrConflict) {
		t.Fatalf("cannot unpublish a draft: %v", err)
	}
	if err = db.QueryRowContext(ctx, "SELECT count(*) FROM starter_product_events WHERE product_id=$1 AND action='unpublish'", draft.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("unpublish audit count %d %v", count, err)
	}
	published = draft
	if err = repo.command(ctx, actor, published.ID, published.Revision, "deleted"); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.get(ctx, actor, published.ID); !errors.Is(err, crud.ErrNotFound) {
		t.Fatalf("delete %v", err)
	}
}

func raceProductCommand(t *testing.T, repo productRepo, actor host.Actor, id, revision, action string) {
	t.Helper()
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Go(func() {
			<-start
			results <- repo.command(t.Context(), actor, id, revision, action)
		})
	}
	close(start)
	workers.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, crud.ErrConflict):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("%s CAS: %d successes, %d conflicts", action, successes, conflicts)
	}
}
