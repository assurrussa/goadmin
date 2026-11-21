package mcp

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/assurrussa/goadmin/host"
)

// expire reconciles a previously observed request whose binding ended. An
// in-flight decision is surfaced immediately, just as for a status read.
func (s actionStore) expire(ctx context.Context, actor host.Actor, id string) (actionRequest, error) {
	return s.run(ctx, func(ctx context.Context, tx pgx.Tx) (actionRequest, error) {
		const query = selectActionRequest + " FOR UPDATE NOWAIT"
		req, err := scanActionRequest(tx.QueryRow(ctx, query, s.action.Namespace, actor.AdminID, id))
		var dbErr *pgconn.PgError
		if errors.As(err, &dbErr) && dbErr.Code == "55P03" {
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
		return s.finish(ctx, tx, req, actionExpired, nil)
	})
}

// sweep expires one bounded batch for this action. When checking bindings the
// caller supplies authoritative liveness for this deployment. A failed check
// leaves that batch unchanged. The callback must not acquire business locks.
func (s actionStore) sweep(
	ctx context.Context, limit int, bindingAlive func(context.Context, ActionTx, actionRequest) (bool, error),
) (int, error) {
	if limit <= 0 || limit > 256 {
		return 0, errActionInvalid
	}
	var count int
	_, err := s.run(ctx, func(ctx context.Context, tx pgx.Tx) (actionRequest, error) {
		requests, err := s.lockSweepBatch(ctx, tx, limit, bindingAlive == nil)
		if err != nil {
			return actionRequest{}, err
		}
		for _, req := range requests {
			now, err := actionClock(ctx, tx)
			if err != nil {
				return actionRequest{}, err
			}
			if now.Before(req.ExpiresAt) {
				if bindingAlive == nil {
					continue
				}
				alive, err := bindingAlive(ctx, tx, req)
				if err != nil {
					return actionRequest{}, err
				}
				if alive {
					continue
				}
			}
			if _, err := s.finish(ctx, tx, req, actionExpired, nil); err != nil {
				return actionRequest{}, err
			}
			count++
		}
		return actionRequest{}, nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (s actionStore) lockSweepBatch(ctx context.Context, tx pgx.Tx, limit int, onlyExpired bool) ([]actionRequest, error) {
	query := "SELECT " + actionColumns + ` FROM admin_confirmed_actions
		WHERE namespace=$1 AND action_key=$2 AND state='pending'`
	args := []any{s.action.Namespace, s.action.Key, limit}
	if onlyExpired {
		now, err := actionClock(ctx, tx)
		if err != nil {
			return nil, err
		}
		args = append(args, now)
		query += " AND expires_at <= $4"
	}
	rows, err := tx.Query(ctx, query+`
		ORDER BY expires_at, actor_admin_id, request_id LIMIT $3 FOR UPDATE SKIP LOCKED`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var requests []actionRequest
	for rows.Next() {
		req, err := scanActionRequest(rows)
		if err != nil {
			return nil, err
		}
		requests = append(requests, req)
	}
	return requests, rows.Err()
}
