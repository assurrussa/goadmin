package host

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// The core owns retention so disabling action routes does not leave private
// review snapshots behind. Concurrent web processes can sweep separate rows.
func confirmedActionExpiryWorker(pool *pgxpool.Pool, log Logger) runtimeWorker {
	const batch = 256
	return runtimeWorker{name: "confirmed action expiry", run: func(ctx context.Context) error {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for ctx.Err() == nil {
			n, err := expireConfirmedActionBatch(ctx, pool, batch)
			if err != nil && ctx.Err() == nil {
				log.ErrorContext(ctx, "Could not expire confirmed action requests", "error", err)
			}
			if err == nil && n == batch {
				continue
			}
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
			}
		}
		return nil
	}}
}

func expireConfirmedActionBatch(ctx context.Context, pool *pgxpool.Pool, limit int) (int64, error) {
	const query = `WITH expired AS (
		SELECT namespace, actor_admin_id, request_id FROM admin_confirmed_actions
		WHERE state='pending' AND expires_at <= statement_timestamp()
		ORDER BY expires_at, namespace, actor_admin_id, request_id LIMIT $1 FOR UPDATE SKIP LOCKED
	)
	UPDATE admin_confirmed_actions a SET state='expired', decided_at=clock_timestamp(),
		normalized_input=NULL, review_fields=NULL, binding_id=NULL
	FROM expired e WHERE (a.namespace,a.actor_admin_id,a.request_id)=(e.namespace,e.actor_admin_id,e.request_id)`
	tag, err := pool.Exec(ctx, query, limit)
	return tag.RowsAffected(), err
}
