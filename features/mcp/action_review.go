package mcp

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/assurrussa/goadmin/host"
)

func (h *actionHandler) loadBrowserReview(ctx context.Context, b actionBrowser, id string) (actionRequest, error) {
	release := func() {}
	defer func() { release() }()
	return h.store.run(ctx, func(ctx context.Context, tx pgx.Tx) (actionRequest, error) {
		r, err := h.store.lockReviewRequest(ctx, tx, b.actor.AdminID, id)
		if err != nil {
			return actionRequest{}, err
		}
		if r.State != actionPending {
			return r, nil
		}
		g, ok := h.grants.bound(r.BindingID)
		if !ok {
			return h.store.finish(ctx, tx, r, actionExpired)
		}
		if g.browser != b.id || g.actor != b.actor {
			return actionRequest{}, errActionNotFound
		}

		var current ActionSnapshot
		var loadErr error
		if r.Version == h.action.Version {
			current, loadErr = h.action.LoadReview(ctx, tx, b.actor, r.input)
		}
		matches := r.Snapshot.RecordID == current.RecordID && r.Snapshot.Revision == current.Revision
		if loadErr == nil && matches {
			loadErr = validateBrowserReview(current.Fields)
		}
		// The transaction and review data are acquired before the login fence,
		// matching the ordering used by confirmation. Recheck before disclosure.
		var ready bool
		ready, release = h.fences.acquire(b.id)
		if !ready {
			return actionRequest{}, errActionTransition
		}
		deadline, err := h.checkGrantInTx(ctx, g)
		if errors.Is(err, host.ErrReadSessionEnded) || errors.Is(err, host.ErrReadSessionDenied) {
			return h.store.finish(ctx, tx, r, actionExpired)
		}
		if err != nil {
			return actionRequest{}, err
		}
		now, err := actionClock(ctx, tx)
		if err != nil {
			return actionRequest{}, err
		}
		switch {
		case !now.Before(r.ExpiresAt) || !now.Before(deadline):
			return h.store.finish(ctx, tx, r, actionExpired)
		case loadErr != nil && !actionRecordMissing(loadErr):
			return actionRequest{}, loadErr
		case loadErr != nil || !matches:
			return h.store.finish(ctx, tx, r, actionConflict)
		}
		r.Snapshot.Fields = current.Fields
		return r, nil
	})
}

func (s actionStore) lockReviewRequest(ctx context.Context, tx pgx.Tx, owner int64, id string) (actionRequest, error) {
	// Use the decision's request lock before taking the row lock.
	const lock = "SELECT pg_try_advisory_xact_lock(hashtextextended($1, 0))"
	var locked bool
	if err := tx.QueryRow(ctx, lock, s.lockKey(owner, id)).Scan(&locked); err != nil {
		return actionRequest{}, err
	}
	if !locked {
		return actionRequest{}, errActionUnknown
	}
	const query = selectActionRequest + " FOR UPDATE NOWAIT"
	r, err := scanActionRequest(tx.QueryRow(ctx, query, s.action.Namespace, owner, id))
	var dbErr *pgconn.PgError
	if errors.As(err, &dbErr) && dbErr.Code == pgLockNotAvailable {
		return actionRequest{}, errActionUnknown
	}
	if err != nil {
		return actionRequest{}, err
	}
	if r.Key != s.action.Key {
		return actionRequest{}, errActionNotFound
	}
	return r, nil
}

func validateBrowserReview(fields []ReviewField) error {
	if len(fields) == 0 || len(fields) > 32 {
		return errors.New("mcp: invalid action browser review")
	}
	for _, field := range fields {
		if !validActionText(field.Label, 200) || (field.Value != "" && !validActionText(field.Value, len(field.Value))) {
			return errors.New("mcp: invalid action browser review field")
		}
	}
	return nil
}
