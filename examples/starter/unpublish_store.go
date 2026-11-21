package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/assurrussa/goadmin/features/crud"
	"github.com/assurrussa/goadmin/host"
)

const (
	unpublishPending     = "pending"
	unpublishSucceeded   = "succeeded"
	unpublishDeclined    = "declined"
	unpublishExpired     = "expired"
	unpublishConflict    = "conflict"
	unpublishMaxLifetime = 15 * time.Minute
)

var (
	errUnpublishInvalid      = errors.New("invalid unpublish request")
	errUnpublishConflict     = errors.New("unpublish request UUID already has a different payload")
	errUnpublishNotFound     = errors.New("unpublish request or eligible product not found")
	errUnpublishExpired      = errors.New("original unpublish binding expired")
	errUnpublishUnauthorized = errors.New("unpublish authorization unavailable")
	errUnpublishUnknown      = crud.ErrUnknown
)

// Product and event identifiers are retained as snapshots when a product is deleted.
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

type unpublishProposal struct {
	RequestID string
	ProductID string
	Reason    string
	BindingID string
	ExpiresAt time.Time
}

type unpublishRequest struct {
	RequestID       string
	ProductID       string
	ProductName     string
	ProductRevision string
	Reason          string
	BindingID       string `json:"-"`
	State           string
	EventID         string
	ExpiresAt       time.Time
}

const unpublishRequestColumns = `
	request_id::text, product_id::text, product_name, product_revision::text, reason, binding_id,
	CASE WHEN state='pending' AND expires_at <= clock_timestamp() THEN 'expired' ELSE state END,
	COALESCE(event_id::text, ''), expires_at`

const (
	selectUnpublishRequest = "SELECT " + unpublishRequestColumns + " FROM starter_product_unpublish_requests WHERE owner_admin_id=$1 AND request_id=$2"
	lockUnpublishProduct   = "SELECT " + productColumns + " FROM starter_products WHERE owner_admin_id=$1 AND id=$2::text::bigint FOR UPDATE"
)

func scanUnpublishRequest(row *sql.Row) (unpublishRequest, error) {
	var req unpublishRequest
	err := row.Scan(&req.RequestID, &req.ProductID, &req.ProductName, &req.ProductRevision,
		&req.Reason, &req.BindingID, &req.State, &req.EventID, &req.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = errUnpublishNotFound
	}
	return req, err
}

func unpublishRequestID(raw string) (string, error) {
	id := strings.ToLower(strings.TrimSpace(raw))
	var uuid pgtype.UUID
	if len(id) != 36 || uuid.Scan(id) != nil || uuid.Bytes == [16]byte{} || uuid.String() != id {
		return "", errUnpublishInvalid
	}
	return id, nil
}

func (r productRepo) proposeUnpublish(ctx context.Context, actor host.Actor, input unpublishProposal, authorize func(context.Context) error) (unpublishRequest, error) {
	if actor.AdminID <= 0 || authorize == nil {
		return unpublishRequest{}, errUnpublishUnauthorized
	}
	requestID, idErr := unpublishRequestID(input.RequestID)
	productID, productErr := strconv.ParseInt(strings.TrimSpace(input.ProductID), 10, 64)
	reason, reasonErr := unpublishReason.Normalize(input.Reason)
	if idErr != nil || productErr != nil || productID <= 0 || reasonErr != nil ||
		input.BindingID == "" || len(input.BindingID) > 256 || !validProductText(input.BindingID) || input.ExpiresAt.IsZero() {
		return unpublishRequest{}, errUnpublishInvalid
	}
	productKey := strconv.FormatInt(productID, 10)

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return unpublishRequest{}, err
	}
	defer tx.Rollback()

	// Serialize absent request keys before reading the product. A replay
	// needs only the retained receipt, even if the product no longer exists.
	lockKey := unpublishLockKey(actor.AdminID, requestID)
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return unpublishRequest{}, err
	}
	req, err := scanUnpublishRequest(tx.QueryRowContext(ctx, selectUnpublishRequest+" FOR UPDATE", actor.AdminID, requestID))
	if err == nil {
		if err = authorize(ctx); err != nil {
			return unpublishRequest{}, err
		}
		if req.ProductID != productKey || req.Reason != reason {
			return unpublishRequest{}, errUnpublishConflict
		}
		// Re-read the expiry after authorization can have waited.
		return scanUnpublishRequest(tx.QueryRowContext(ctx, selectUnpublishRequest, actor.AdminID, requestID))
	}
	if !errors.Is(err, errUnpublishNotFound) {
		return unpublishRequest{}, err
	}

	product, productErr := productScan(tx.QueryRowContext(ctx, lockUnpublishProduct, actor.AdminID, productKey))
	if err = authorize(ctx); err != nil {
		return unpublishRequest{}, err
	}
	if errors.Is(productErr, crud.ErrNotFound) || (productErr == nil && product.Data.Status != "published") {
		return unpublishRequest{}, errUnpublishNotFound
	}
	if productErr != nil {
		return unpublishRequest{}, productErr
	}

	// PostgreSQL timestamps have microsecond precision. Truncate the
	// externally supplied deadline so the stored value cannot extend it.
	expiresAt := input.ExpiresAt.Truncate(time.Microsecond)
	var now time.Time
	if err = tx.QueryRowContext(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil {
		return unpublishRequest{}, err
	}
	if !expiresAt.After(now) {
		return unpublishRequest{}, errUnpublishExpired
	}
	if expiresAt.After(now.Add(unpublishMaxLifetime)) {
		return unpublishRequest{}, errUnpublishInvalid
	}
	req, err = scanUnpublishRequest(tx.QueryRowContext(ctx, `
		INSERT INTO starter_product_unpublish_requests
			(owner_admin_id, request_id, product_id, product_name, product_revision, reason, binding_id, created_at, expires_at, state)
		VALUES ($1, $2, $3, $4, $5::text::bigint, $6, $7, $8, $9, 'pending')
		RETURNING `+unpublishRequestColumns,
		actor.AdminID, requestID, productID, product.Data.Name, product.Revision, reason, input.BindingID, now, expiresAt,
	))
	if err != nil {
		return unpublishRequest{}, err
	}
	if err := commitUnpublish(tx); err != nil {
		return unpublishRequest{}, err
	}
	return req, nil
}

func unpublishLockKey(owner int64, id string) string {
	return fmt.Sprintf("starter-product-unpublish/%d/%s", owner, id)
}

func (r productRepo) lookupUnpublish(ctx context.Context, actor host.Actor, requestID string) (unpublishRequest, error) {
	if actor.AdminID <= 0 {
		return unpublishRequest{}, errUnpublishUnauthorized
	}
	id, err := unpublishRequestID(requestID)
	if err != nil {
		return unpublishRequest{}, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return unpublishRequest{}, err
	}
	defer tx.Rollback()
	// A proposal in flight may not yet have a visible row. Serialize absence
	// with creation, and the receipt itself with an in-flight decision.
	var ready bool
	if err := tx.QueryRowContext(ctx, "SELECT pg_try_advisory_xact_lock_shared(hashtextextended($1, 0))", unpublishLockKey(actor.AdminID, id)).Scan(&ready); err != nil {
		return unpublishRequest{}, err
	}
	if !ready {
		return unpublishRequest{}, errUnpublishUnknown
	}
	req, err := scanUnpublishRequest(tx.QueryRowContext(ctx, selectUnpublishRequest+" FOR SHARE NOWAIT", actor.AdminID, id))
	var dbErr *pgconn.PgError
	if errors.As(err, &dbErr) && dbErr.Code == "55P03" {
		return unpublishRequest{}, errUnpublishUnknown
	}
	return req, err
}

func (r productRepo) decideUnpublish(ctx context.Context, actor host.Actor, requestID string, confirm bool, authorize func(context.Context, *sql.Tx, unpublishRequest) error) (unpublishRequest, error) {
	if actor.AdminID <= 0 || authorize == nil {
		return unpublishRequest{}, errUnpublishUnauthorized
	}
	id, err := unpublishRequestID(requestID)
	if err != nil {
		return unpublishRequest{}, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return unpublishRequest{}, err
	}
	defer tx.Rollback()

	req, err := scanUnpublishRequest(tx.QueryRowContext(ctx, selectUnpublishRequest+" FOR UPDATE", actor.AdminID, id))
	if err != nil || req.State != unpublishPending {
		return req, err
	}
	productMatches := false
	if confirm {
		product, err := productScan(tx.QueryRowContext(ctx, lockUnpublishProduct, actor.AdminID, req.ProductID))
		if err != nil && !errors.Is(err, crud.ErrNotFound) {
			return unpublishRequest{}, err
		}
		productMatches = err == nil && product.Revision == req.ProductRevision && product.Data.Status == "published"
	}
	// Browser authorization can invalidate the exact original binding
	// permanently, or return an error that preserves the pending request.
	authErr := authorize(ctx, tx, req)
	var expired bool
	if err = tx.QueryRowContext(ctx, "SELECT $1::timestamptz <= clock_timestamp()", req.ExpiresAt).Scan(&expired); err != nil {
		return unpublishRequest{}, err
	}
	switch {
	case expired || errors.Is(authErr, errUnpublishExpired):
		req.State = unpublishExpired
	case authErr != nil:
		return unpublishRequest{}, authErr
	case !confirm:
		req.State = unpublishDeclined
	case !productMatches:
		req.State = unpublishConflict
	default:
		// The SQL cutoff closes the gap between the last authorization check
		// and the conditional product mutation.
		var productID string
		err := tx.QueryRowContext(ctx, unpublishProductUpdate+" AND $4::timestamptz > clock_timestamp() RETURNING id::text",
			actor.AdminID, req.ProductID, req.ProductRevision, req.ExpiresAt).Scan(&productID)
		if errors.Is(err, sql.ErrNoRows) {
			req.State = unpublishExpired
			break
		}
		if err != nil {
			return unpublishRequest{}, err
		}
		if err = tx.QueryRowContext(ctx, productAuditInsert+" RETURNING id::text",
			actor.AdminID, productID, "unpublish", req.Reason).Scan(&req.EventID); err != nil {
			return unpublishRequest{}, err
		}
		req.State = unpublishSucceeded
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE starter_product_unpublish_requests SET state=$3, event_id=NULLIF($4, '')::bigint
		WHERE owner_admin_id=$1 AND request_id=$2`, actor.AdminID, id, req.State, req.EventID); err != nil {
		return unpublishRequest{}, err
	}
	if err := commitUnpublish(tx); err != nil {
		return unpublishRequest{}, err
	}
	return req, nil
}

func commitUnpublish(tx *sql.Tx) error {
	err := tx.Commit()
	if err == nil || errors.Is(err, pgx.ErrTxCommitRollback) {
		return err
	}
	var serverError *pgconn.PgError
	if errors.As(err, &serverError) &&
		(strings.HasPrefix(serverError.Code, "23") || serverError.Code == "40001" || serverError.Code == "40P01") {
		return err
	}
	// A transport failure can occur after PostgreSQL made the receipt durable.
	// The caller reconciles with a read of the same UUID.
	return fmt.Errorf("%w: %w", errUnpublishUnknown, err)
}
