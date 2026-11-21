package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"time"
)

// Version 4 is the immutable starter pilot schema.
const createUnpublishRequests = `
CREATE TABLE starter_product_unpublish_requests (
	owner_admin_id bigint NOT NULL,
	request_id uuid NOT NULL,
	product_id bigint NOT NULL,
	product_name text NOT NULL,
	product_revision bigint NOT NULL,
	reason text NOT NULL CHECK (length(reason) BETWEEN 1 AND 500),
	binding_id text NOT NULL CHECK (length(binding_id) BETWEEN 1 AND 256),
	created_at timestamptz NOT NULL,
	expires_at timestamptz NOT NULL CHECK (expires_at > created_at AND expires_at <= created_at + interval '15 minutes'),
	state text NOT NULL CHECK (state IN ('pending', 'succeeded', 'declined', 'expired', 'conflict')),
	event_id bigint,
	PRIMARY KEY (owner_admin_id, request_id),
	CHECK ((state = 'succeeded') = (event_id IS NOT NULL))
)`

// Earlier proposals have no portable native grant. Preserve their UUIDs and
// receipts so restarting on the new implementation cannot dispatch them.
func migrateUnpublishHistory(ctx context.Context, tx *sql.Tx) error {
	const pageSize = 128
	var lastOwner int64
	lastID := "00000000-0000-0000-0000-000000000000"
	type legacy struct {
		owner                             int64
		uuid, id, revision, reason, state string
		event                             sql.NullString
		created, expires                  time.Time
		decided                           sql.NullTime
	}
	for {
		rows, err := tx.QueryContext(ctx, `SELECT r.owner_admin_id,r.request_id::text,r.product_id::text,r.product_revision::text,
			r.reason,r.state,r.event_id::text,r.created_at,r.expires_at,e.created_at
			FROM starter_product_unpublish_requests r LEFT JOIN starter_product_events e ON e.id=r.event_id AND e.owner_admin_id=r.owner_admin_id
			WHERE (r.owner_admin_id,r.request_id)>($1,$2::uuid)
			ORDER BY r.owner_admin_id,r.request_id LIMIT $3`, lastOwner, lastID, pageSize)
		if err != nil {
			return err
		}
		batch := make([]legacy, 0, pageSize)
		for rows.Next() {
			var x legacy
			if err = rows.Scan(&x.owner, &x.uuid, &x.id, &x.revision, &x.reason, &x.state, &x.event, &x.created, &x.expires, &x.decided); err != nil {
				rows.Close()
				return err
			}
			batch = append(batch, x)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, x := range batch {
			payload, err := json.Marshal(withdrawalInput{ProductID: x.id, Reason: x.reason})
			if err != nil {
				return err
			}
			fingerprint := sha256.Sum256(payload)
			var result []byte
			if x.event.Valid {
				result, err = json.Marshal(map[string]string{"event_id": x.event.String})
				if err != nil {
					return err
				}
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO admin_confirmed_actions(namespace,actor_admin_id,request_id,action_key,action_version,
				input_fingerprint,record_id,record_revision,created_at,expires_at,state,result,decided_at)
				VALUES('starter',$1,$2,'product-unpublish','1',$3,$4,$5,$6,$7,
				CASE WHEN $8='pending' THEN 'expired' ELSE $8 END,$9,
				CASE WHEN $8='pending' THEN LEAST(clock_timestamp(),$7) ELSE $10::timestamptz END)`,
				x.owner, x.uuid, fingerprint[:], x.id, x.revision, x.created, x.expires, x.state, result, x.decided)
			if err != nil {
				return err
			}
			lastOwner, lastID = x.owner, x.uuid
		}
		if len(batch) < pageSize {
			return nil
		}
	}
}
