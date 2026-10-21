//go:build integration

package sso //nolint:testpackage // Durable intent regressions.
import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/assurrussa/goauth"

	"github.com/assurrussa/goadmin/host"
)

const testNativeOne = "native-one"

func TestPostgresLinkIntentIsolationReplayAndConcurrency(t *testing.T) {
	db := testSSODatabase(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	if err := MigrateSchema(ctx, db, "seo_sso"); err != nil {
		t.Fatal(err)
	}
	store := postgresIntents{db: db, table: `"seo_sso".sso_link_intents`}
	binding := host.ExternalBinding{
		LocalSubject: goauth.NewSubjectID(),
		Deadline:     time.Now().Add(time.Minute).UTC().Truncate(time.Microsecond),
	}
	var err error
	secret := strings.Repeat("x", 43)
	if err = store.create(ctx, secret, testNativeOne, binding); err != nil {
		t.Fatal(err)
	}
	if err = MigrateSchema(ctx, db, "seo_sso"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.consume(ctx, secret, "native-two", binding.LocalSubject); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("wrong browser consumed: %v", err)
	}
	if _, err = store.consume(ctx, secret, testNativeOne, goauth.NewSubjectID()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("wrong subject consumed: %v", err)
	}
	// Concurrent confirmations must have exactly one committed winner.
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			got, err := store.consume(ctx, secret, testNativeOne, binding.LocalSubject)
			if err == nil {
				successes.Add(1)
				if got != binding {
					t.Error("durable intent changed immutable binding")
				}
			} else if !errors.Is(err, sql.ErrNoRows) {
				t.Errorf("concurrent consume: %v", err)
			}
		})
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("confirmation committed %d times", successes.Load())
	}
	if _, err = store.consume(ctx, secret, testNativeOne, binding.LocalSubject); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("replay accepted: %v", err)
	}
	// Expired proofs cannot be linked even by a current native administrator.
	binding.Deadline = time.Now().Add(-time.Second)
	if err = store.create(ctx, "expired-intent", testNativeOne, binding); err != nil {
		t.Fatal(err)
	}
	if _, err = store.consume(ctx, "expired-intent", testNativeOne, binding.LocalSubject); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expired intent consumed: %v", err)
	}
	var stored string
	query := `SELECT id_digest FROM ` + store.table + ` WHERE native_binding=$1 AND consumed_at IS NOT NULL`
	if err = db.QueryRowContext(ctx, query, testNativeOne).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == secret || stored != intentDigest(secret) {
		t.Fatal("raw confirmation secret persisted")
	}
}

func TestNavigationCleanupIsBoundedAndPreservesLiveMarkers(t *testing.T) {
	db := testSSODatabase(t)
	ctx := t.Context()
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	_, err := db.ExecContext(ctx, `INSERT INTO goadmin_sso.sso_login_return_intents(state_digest,target,expires_at)
 SELECT 'expired-'||i,'/private',clock_timestamp()-interval '1 minute' FROM generate_series(1,200) i;
 INSERT INTO goadmin_sso.sso_login_return_intents(state_digest,target,expires_at,consumed_at)
 VALUES ('live','/live',clock_timestamp()+interval '1 hour',NULL),
 ('consumed','/used',clock_timestamp()+interval '1 hour',clock_timestamp());`)
	if err != nil {
		t.Fatal(err)
	}
	rp := &Runtime{db: db}
	if err = rp.pruneNavigation(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM goadmin_sso.sso_login_return_intents`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 74 {
		t.Fatalf("cleanup removed more than 128 or live markers: %d", count)
	}
	if err = rp.pruneNavigation(ctx); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM goadmin_sso.sso_login_return_intents`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("live replay markers removed: %d", count)
	}
}
