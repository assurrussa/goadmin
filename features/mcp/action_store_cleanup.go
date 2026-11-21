package mcp

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/assurrussa/goadmin/host"
)

func (s actionStore) expireBindings(ctx context.Context, retired []actionGrant) error {
	owners := make([]int64, 0, len(retired))
	bindings := make([]string, 0, len(retired))
	for _, grant := range retired {
		owners = append(owners, grant.actor.AdminID)
		bindings = append(bindings, grant.id)
	}
	_, err := s.run(ctx, func(ctx context.Context, tx pgx.Tx) (actionRequest, error) {
		_, err := tx.Exec(ctx, `
			WITH pending AS (
				SELECT actor_admin_id, request_id FROM admin_confirmed_actions
				JOIN unnest($3::bigint[], $4::text[]) AS retired(admin_id, binding)
					ON actor_admin_id=retired.admin_id AND binding_id=retired.binding
				WHERE namespace=$1 AND action_key=$2 AND state='pending'
				ORDER BY actor_admin_id, request_id FOR UPDATE OF admin_confirmed_actions
			)
			UPDATE admin_confirmed_actions a SET state='expired', decided_at=clock_timestamp(),
				normalized_input=NULL, review_fields=NULL, binding_id=NULL
			FROM pending p WHERE a.namespace=$1 AND (a.actor_admin_id,a.request_id)=(p.actor_admin_id,p.request_id)`,
			s.action.Namespace, s.action.Key, owners, bindings)
		return actionRequest{}, err
	})
	return err
}

// expire reconciles a previously observed request whose binding ended. An
// in-flight decision is surfaced immediately, just as for a status read.
func (s actionStore) expire(ctx context.Context, actor host.Actor, id string) (actionRequest, error) {
	return s.run(ctx, func(ctx context.Context, tx pgx.Tx) (actionRequest, error) {
		const query = selectActionRequest + " FOR UPDATE NOWAIT"
		req, err := scanActionRequest(tx.QueryRow(ctx, query, s.action.Namespace, actor.AdminID, id))
		var dbErr *pgconn.PgError
		if errors.As(err, &dbErr) && dbErr.Code == pgLockNotAvailable {
			return actionRequest{}, errActionUnknown
		}
		if err != nil {
			return actionRequest{}, err
		}
		if req.Key != s.action.Key {
			return actionRequest{}, errActionNotFound
		}
		if req.State != actionPending {
			return req, nil
		}
		return s.finish(ctx, tx, req, actionExpired)
	})
}
