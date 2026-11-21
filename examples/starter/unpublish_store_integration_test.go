//go:build integration

package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/assurrussa/goadmin/features/crud"
	"github.com/assurrussa/goadmin/host"
)

func testUnpublishProduct(t *testing.T, repo productRepo, owner host.Actor, name string) string {
	t.Helper()
	product, err := repo.create(t.Context(), owner, productInput{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.command(t.Context(), owner, product.ID, product.Revision, "publish"); err != nil {
		t.Fatal(err)
	}
	return product.ID
}

func testUnpublishProposal(productID string, key int) unpublishProposal {
	return unpublishProposal{
		RequestID: fmt.Sprintf("00000000-0000-4000-8000-%012x", key), ProductID: productID,
		Reason: "Withdrawing the old listing", BindingID: "original-connection",
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
}

func testAllowProposal(context.Context) error                            { return nil }
func testAllowDecision(context.Context, *sql.Tx, unpublishRequest) error { return nil }

func TestUnpublishPostgresConcurrentReceiptAndOwnerIsolation(t *testing.T) {
	db, dsn, owner := productTestDatabase(t, "products")
	repo := productRepo{db}
	ctx := t.Context()
	other := productTestActor(t, db, "00000000-0000-4000-8000-000000000002")
	productID := testUnpublishProduct(t, repo, owner, "Original name")
	otherProduct := testUnpublishProduct(t, repo, other, "Owned by other operator")
	input := testUnpublishProposal(productID, 10)
	invalidReason := input
	invalidReason.Reason = " \t\n"
	if _, err := repo.proposeUnpublish(ctx, owner, invalidReason, testAllowProposal); !errors.Is(err, errUnpublishInvalid) {
		t.Fatalf("accepted a blank reason: %v", err)
	}
	excessLifetime := input
	excessLifetime.ExpiresAt = time.Now().Add(16 * time.Minute)
	if _, err := repo.proposeUnpublish(ctx, owner, excessLifetime, testAllowProposal); !errors.Is(err, errUnpublishInvalid) {
		t.Fatalf("accepted a binding lifetime over 15 minutes: %v", err)
	}
	normalized := input
	normalized.RequestID = strings.ToUpper(input.RequestID)
	normalized.ProductID = " 0" + input.ProductID
	normalized.Reason = "\n  " + input.Reason + "\t"
	normalized.BindingID = "another-current-binding"
	normalized.ExpiresAt = input.ExpiresAt.Add(time.Minute)

	var proposal [2]unpublishRequest
	testUnpublishConcurrently(t, func(index int) error {
		arg := [2]unpublishProposal{input, normalized}[index]
		var err error
		proposal[index], err = repo.proposeUnpublish(ctx, owner, arg, testAllowProposal)
		return err
	})
	req := proposal[0]
	if req != proposal[1] || req.State != unpublishPending || req.ProductID != productID ||
		req.ProductName != "Original name" || req.ProductRevision != "2" ||
		req.Reason != input.Reason || req.EventID != "" {
		t.Fatalf("proposal identity or snapshot changed: %+v vs %+v", req, proposal[1])
	}
	later := input
	later.BindingID = "a-fresh-connection"
	later.ExpiresAt = input.ExpiresAt.Add(2 * time.Minute)
	if replay, err := repo.proposeUnpublish(ctx, owner, later, testAllowProposal); err != nil || replay != req {
		t.Fatalf("pending proposal was assigned a different binding or deadline: %+v / %v", replay, err)
	}

	for _, conflicting := range []unpublishProposal{
		{input.RequestID, productID, "Different explanation", input.BindingID, input.ExpiresAt},
		{input.RequestID, otherProduct, input.Reason, input.BindingID, input.ExpiresAt},
	} {
		if _, err := repo.proposeUnpublish(ctx, owner, conflicting, testAllowProposal); !errors.Is(err, errUnpublishConflict) {
			t.Fatalf("payload reuse: %v", err)
		}
	}
	if _, err := repo.lookupUnpublish(ctx, other, req.RequestID); !errors.Is(err, errUnpublishNotFound) {
		t.Fatalf("foreign lookup: %v", err)
	}
	if _, err := repo.decideUnpublish(ctx, other, req.RequestID, true, testAllowDecision); !errors.Is(err, errUnpublishNotFound) {
		t.Fatalf("foreign decision: %v", err)
	}
	if _, err := repo.proposeUnpublish(ctx, other, input, testAllowProposal); !errors.Is(err, errUnpublishNotFound) {
		t.Fatalf("foreign product proposal: %v", err)
	}
	otherInput := input
	otherInput.ProductID = otherProduct
	if otherReq, err := repo.proposeUnpublish(ctx, other, otherInput, testAllowProposal); err != nil || otherReq.ProductID != otherProduct {
		t.Fatalf("owner-scoped UUID: %+v / %v", otherReq, err)
	}

	var decisions [2]unpublishRequest
	var authorizationCount atomic.Int32
	testUnpublishConcurrently(t, func(index int) error {
		var err error
		decisions[index], err = repo.decideUnpublish(ctx, owner, req.RequestID, true, func(_ context.Context, _ *sql.Tx, actual unpublishRequest) error {
			if actual != req {
				return fmt.Errorf("authorization got stale request: %+v", actual)
			}
			authorizationCount.Add(1)
			return nil
		})
		return err
	})
	receipt := decisions[0]
	if receipt != decisions[1] || receipt.State != unpublishSucceeded || receipt.EventID == "" || authorizationCount.Load() != 1 {
		t.Fatalf("duplicate decision did not share a receipt: %+v vs %+v", receipt, decisions[1])
	}
	testUnpublishMutation(t, repo, owner, productID, "draft", "3", 1)
	var reason string
	if err := db.QueryRowContext(ctx, "SELECT reason FROM starter_product_events WHERE id::text=$1 AND product_id=$2", receipt.EventID, productID).Scan(&reason); err != nil || reason != input.Reason {
		t.Fatalf("audit receipt: reason=%q err=%v", reason, err)
	}
	if err := repo.command(ctx, owner, productID, "3", "deleted"); err != nil {
		t.Fatal(err)
	}
	reopened, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	repo = productRepo{reopened}
	if lookup, err := repo.lookupUnpublish(ctx, owner, req.RequestID); err != nil || lookup != receipt {
		t.Fatalf("reconciliation after deletion and reopening: %+v / %v", lookup, err)
	}
	if replay, err := repo.proposeUnpublish(ctx, owner, normalized, testAllowProposal); err != nil || replay != receipt {
		t.Fatalf("proposal replay after deletion and reopening: %+v / %v", replay, err)
	}
	if replay, err := repo.decideUnpublish(ctx, owner, req.RequestID, false, testAllowDecision); err != nil || replay != receipt {
		t.Fatalf("a terminal outcome changed: %+v / %v", replay, err)
	}
}

func testUnpublishConcurrently(t *testing.T, operation func(int) error) {
	t.Helper()
	var group sync.WaitGroup
	errs := make(chan error, 2)
	for index := range 2 {
		group.Go(func() { errs <- operation(index) })
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func testUnpublishMutation(t *testing.T, repo productRepo, actor host.Actor, productID, status, revision string, count int) {
	t.Helper()
	product, err := repo.get(t.Context(), actor, productID)
	if err != nil || product.Data.Status != status || product.Revision != revision {
		t.Fatalf("product status or revision: %+v / %v", product, err)
	}
	var actual int
	if err = repo.db.QueryRowContext(t.Context(), "SELECT count(*) FROM starter_product_events WHERE owner_admin_id=$1 AND product_id=$2 AND action='unpublish'", actor.AdminID, productID).Scan(&actual); err != nil || actual != count {
		t.Fatalf("unpublish event count: %d / %v", actual, err)
	}
}

func TestUnpublishPostgresDeclineStaleAndAuthorization(t *testing.T) {
	db, _, owner := productTestDatabase(t, "products")
	repo := productRepo{db}
	ctx := t.Context()
	productID := testUnpublishProduct(t, repo, owner, "Review")
	input := testUnpublishProposal(productID, 20)
	if _, err := repo.proposeUnpublish(ctx, owner, input, testAllowProposal); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.decideUnpublish(ctx, owner, input.RequestID, true, func(context.Context, *sql.Tx, unpublishRequest) error {
		return errUnpublishUnauthorized
	}); !errors.Is(err, errUnpublishUnauthorized) {
		t.Fatalf("unexpected authorization result: %v", err)
	}
	if req, err := repo.lookupUnpublish(ctx, owner, input.RequestID); err != nil || req.State != unpublishPending {
		t.Fatalf("denial modified pending request: %+v / %v", req, err)
	}
	decline, err := repo.decideUnpublish(ctx, owner, input.RequestID, false, testAllowDecision)
	if err != nil || decline.State != unpublishDeclined || decline.EventID != "" {
		t.Fatalf("decline: %+v / %v", decline, err)
	}
	if declineReplay, err := repo.decideUnpublish(ctx, owner, input.RequestID, true, testAllowDecision); err != nil || declineReplay != decline {
		t.Fatalf("attempted confirm after decline: %+v / %v", declineReplay, err)
	}
	testUnpublishMutation(t, repo, owner, productID, "published", "2", 0)

	stale := testUnpublishProposal(productID, 21)
	if _, err := repo.proposeUnpublish(ctx, owner, stale, testAllowProposal); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.update(ctx, owner, productID, "2", productInput{Name: "Updated after proposal"}); err != nil {
		t.Fatal(err)
	}
	result, err := repo.decideUnpublish(ctx, owner, stale.RequestID, true, testAllowDecision)
	if err != nil || result.State != unpublishConflict || result.EventID != "" || result.ProductName != "Review" {
		t.Fatalf("stale result: %+v / %v", result, err)
	}
	testUnpublishMutation(t, repo, owner, productID, "published", "3", 0)

	lost := testUnpublishProposal(productID, 22)
	if _, err := repo.proposeUnpublish(ctx, owner, lost, testAllowProposal); err != nil {
		t.Fatal(err)
	}
	result, err = repo.decideUnpublish(ctx, owner, lost.RequestID, true, func(context.Context, *sql.Tx, unpublishRequest) error { return errUnpublishExpired })
	if err != nil || result.State != unpublishExpired {
		t.Fatalf("lost original binding: %+v / %v", result, err)
	}
	testUnpublishMutation(t, repo, owner, productID, "published", "3", 0)

	deleted := testUnpublishProposal(productID, 23)
	if _, err := repo.proposeUnpublish(ctx, owner, deleted, testAllowProposal); err != nil {
		t.Fatal(err)
	}
	if err := repo.command(ctx, owner, productID, "3", "deleted"); err != nil {
		t.Fatal(err)
	}
	if result, err = repo.decideUnpublish(ctx, owner, deleted.RequestID, true, testAllowDecision); err != nil || result.State != unpublishConflict {
		t.Fatalf("deleted product had an approvable request: %+v / %v", result, err)
	}
}

func TestUnpublishPostgresExpiryAfterProductWait(t *testing.T) {
	db, _, owner := productTestDatabase(t, "products")
	repo := productRepo{db}
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	productID := testUnpublishProduct(t, repo, owner, "Expiring")
	input := testUnpublishProposal(productID, 30)
	input.ExpiresAt = time.Now().Add(2 * time.Second)
	if _, err := repo.proposeUnpublish(ctx, owner, input, testAllowProposal); err != nil {
		t.Fatal(err)
	}
	locker, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer locker.Rollback()
	if _, err = locker.ExecContext(ctx, lockUnpublishProduct, owner.AdminID, productID); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		got, err := repo.decideUnpublish(ctx, owner, input.RequestID, true, testAllowDecision)
		if err == nil && (got.State != unpublishExpired || got.EventID != "") {
			err = fmt.Errorf("confirmation after waiting past expiry: %+v", got)
		}
		result <- err
	}()
	for {
		var waiting bool
		err = db.QueryRowContext(ctx, `
			SELECT EXISTS (SELECT 1 FROM pg_stat_activity
				WHERE datname=current_database() AND wait_event_type='Lock' AND query=$1)`, lockUnpublishProduct).Scan(&waiting)
		if err != nil || time.Now().After(input.ExpiresAt) {
			t.Fatalf("decision did not reach the row lock before expiry: %v", err)
		}
		if waiting {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, err = db.ExecContext(ctx, "SELECT pg_sleep_until($1)", input.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	if err := locker.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if req, err := repo.lookupUnpublish(ctx, owner, input.RequestID); err != nil || req.State != unpublishExpired {
		t.Fatalf("expired receipt: %+v / %v", req, err)
	}
	testUnpublishMutation(t, repo, owner, productID, "published", "2", 0)
}

func TestUnpublishPostgresAuditRollbackAndLostCommitResponse(t *testing.T) {
	db, dsn, owner := productTestDatabase(t, "products")
	repo := productRepo{db}
	ctx := t.Context()
	productID := testUnpublishProduct(t, repo, owner, "Atomic")
	input := testUnpublishProposal(productID, 40)
	creating, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer creating.Rollback()
	if _, err := creating.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", unpublishLockKey(owner.AdminID, input.RequestID)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.lookupUnpublish(ctx, owner, input.RequestID); !errors.Is(err, crud.ErrUnknown) {
		t.Fatalf("lookup during first creation returned %v", err)
	}
	if err := creating.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.lookupUnpublish(ctx, owner, input.RequestID); !errors.Is(err, errUnpublishNotFound) {
		t.Fatalf("lookup after rolled back creation returned %v", err)
	}
	if _, err := repo.proposeUnpublish(ctx, owner, input, testAllowProposal); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "ALTER TABLE starter_product_events ADD CONSTRAINT pause_unpublish_audit CHECK (action <> 'unpublish')"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.decideUnpublish(ctx, owner, input.RequestID, true, testAllowDecision); err == nil || errors.Is(err, crud.ErrUnknown) {
		t.Fatalf("audit failure should report a known rollback: %v", err)
	}
	if got, err := repo.lookupUnpublish(ctx, owner, input.RequestID); err != nil || got.State != unpublishPending || got.EventID != "" {
		t.Fatalf("audit failure left a changed receipt: %+v / %v", got, err)
	}
	testUnpublishMutation(t, repo, owner, productID, "published", "2", 0)
	inFlight, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer inFlight.Rollback()
	if _, err := inFlight.ExecContext(ctx, selectUnpublishRequest+" FOR UPDATE", owner.AdminID, input.RequestID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.lookupUnpublish(ctx, owner, input.RequestID); !errors.Is(err, crud.ErrUnknown) {
		t.Fatalf("lookup during an undecided commit returned %v", err)
	}
	if err := inFlight.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "ALTER TABLE starter_product_events DROP CONSTRAINT pause_unpublish_audit"); err != nil {
		t.Fatal(err)
	}

	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	fault := &unpublishLostCommitConnector{Connector: stdlib.GetConnector(*config)}
	faultDB := sql.OpenDB(fault)
	defer faultDB.Close()
	faultRepo := productRepo{faultDB}
	if got, err := faultRepo.decideUnpublish(ctx, owner, input.RequestID, true, testAllowDecision); !errors.Is(err, crud.ErrUnknown) || got != (unpublishRequest{}) {
		t.Fatalf("lost COMMIT response: %+v / %v", got, err)
	}
	if commits := fault.commits.Load(); commits != 1 {
		t.Fatalf("decision attempted %d commits", commits)
	}
	if got, err := repo.lookupUnpublish(ctx, owner, input.RequestID); err != nil || got.State != unpublishSucceeded || got.EventID == "" {
		t.Fatalf("reconciled unknown result: %+v / %v", got, err)
	}
	testUnpublishMutation(t, repo, owner, productID, "draft", "3", 1)
}

type unpublishLostCommitConnector struct {
	driver.Connector
	commits atomic.Int32
}

func (c *unpublishLostCommitConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return unpublishLostCommitConn{conn.(*stdlib.Conn), &c.commits}, nil
}

type unpublishLostCommitConn struct {
	*stdlib.Conn
	commits *atomic.Int32
}

func (c unpublishLostCommitConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	tx, err := c.Conn.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return unpublishLostCommitTx{tx, c.commits}, nil
}

type unpublishLostCommitTx struct {
	driver.Tx
	commits *atomic.Int32
}

func (tx unpublishLostCommitTx) Commit() error {
	tx.commits.Add(1)
	if err := tx.Tx.Commit(); err != nil {
		return err
	}
	return errors.New("test connection lost after PostgreSQL committed")
}
