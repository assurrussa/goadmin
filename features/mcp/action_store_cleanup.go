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
