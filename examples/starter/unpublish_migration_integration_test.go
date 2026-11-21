//go:build integration

package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestPilotWithdrawalUpgradePreservesReceipts(t *testing.T) {
	db, dsn, actor := productTestDatabase(t, "products")
	ctx := t.Context()
	if _, err := db.ExecContext(ctx, "DELETE FROM starter_product_migrations WHERE version=5"); err != nil {
		t.Fatal(err)
	}
	const reason = "Архив <>&\u2028🙂"
	var event string
	var decision time.Time
	if err := db.QueryRowContext(ctx, `INSERT INTO starter_product_events(owner_admin_id,product_id,action,reason)
		VALUES($1,'43','unpublish',$2) RETURNING id::text,created_at`, actor.AdminID, reason).Scan(&event, &decision); err != nil {
		t.Fatal(err)
	}
	states := []string{"succeeded", "declined", "conflict", "expired", "pending"}
	for i, state := range states {
		var eventID any
		if state == "succeeded" {
			eventID = event
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO starter_product_unpublish_requests
			(owner_admin_id,request_id,product_id,product_name,product_revision,reason,binding_id,created_at,expires_at,state,event_id)
			VALUES($1,$2,43,'Legacy product',7,$3,'old-login',now(),now()+interval '10 minutes',$4,$5::text::bigint)`,
			actor.AdminID, fmt.Sprintf("00000000-0000-4000-8000-%012d", i+1), reason, state, eventID); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := migrateProducts(ctx, dsn); err != nil {
			t.Fatal(err)
		}
	}
	input, err := normalizeWithdrawal("1", map[string]json.RawMessage{"product_id": json.RawMessage(`"43"`), "reason": mustJSONString(t, reason)})
	if err != nil {
		t.Fatal(err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(input))
	for i, state := range states {
		var stored, legacy, fingerprint string
		var result sql.NullString
		var when sql.NullTime
		err := db.QueryRowContext(ctx, `SELECT n.state,o.state,encode(n.input_fingerprint,'hex'),n.result->>'event_id',n.decided_at
			FROM admin_confirmed_actions n JOIN starter_product_unpublish_requests o ON o.owner_admin_id=n.actor_admin_id AND o.request_id=n.request_id
			WHERE n.namespace='starter' AND n.action_key='product-unpublish' AND n.action_version='1'
			AND n.normalized_input IS NULL AND n.review_fields IS NULL AND n.binding_id IS NULL AND n.record_id='43' AND n.record_revision='7'
			AND n.actor_admin_id=$1 AND n.request_id=$2`, actor.AdminID, fmt.Sprintf("00000000-0000-4000-8000-%012d", i+1)).
			Scan(&stored, &legacy, &fingerprint, &result, &when)
		want := state
		if state == "pending" {
			want = "expired"
		}
		if err != nil || fingerprint != hash || legacy != state || stored != want ||
			result.Valid != (state == "succeeded") || (result.Valid && (result.String != event || !when.Time.Equal(decision))) ||
			when.Valid != (state == "pending" || state == "succeeded") {
			t.Fatalf("import %s: status=%s legacy=%s result=%v time=%v hash match=%v err=%v", state, stored, legacy, result, when, fingerprint == hash, err)
		}
	}
}

func mustJSONString(t *testing.T, s string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
