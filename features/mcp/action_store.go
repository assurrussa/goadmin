package mcp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	pgstorage "github.com/assurrussa/outbox/backends/pgsql/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/assurrussa/goadmin/features/crud"
	"github.com/assurrussa/goadmin/host"
)

const (
	actionPending     = "pending"
	actionSucceeded   = "succeeded"
	actionDeclined    = "declined"
	actionExpired     = "expired"
	actionConflict    = "conflict"
	actionMaxLifetime = 15 * time.Minute
)

var (
	errActionInvalid      = errors.New("mcp: invalid action request")
	errActionConflict     = errors.New("mcp: action request UUID already has a different payload")
	errActionNotFound     = ErrActionNotFound
	errActionExpired      = errors.New("mcp: action's original binding ended")
	errActionUnauthorized = errors.New("mcp: action authorization unavailable")
	errActionUnknown      = errors.New("mcp: action outcome unknown")
)

type actionStore struct {
	tx     host.StoragePgsqlTxManager
	action Action
}

type actionProposal struct {
	RequestID string
	Input     map[string]json.RawMessage
	BindingID string
	ExpiresAt time.Time
}

type actionRequest struct {
	RequestID    string
	OwnerAdminID int64
	Key          string
	Version      string
	Snapshot     ActionSnapshot
	BindingID    string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	DecidedAt    time.Time
	State        string
	Result       map[string]string

	input       json.RawMessage
	fingerprint []byte
}

const actionColumns = `request_id::text, actor_admin_id, action_key, action_version,
	input_fingerprint, normalized_input, record_id, record_revision, review_fields,
	binding_id, created_at, expires_at, decided_at, state, result`

const selectActionRequest = "SELECT " + actionColumns + ` FROM admin_confirmed_actions
	WHERE namespace=$1 AND actor_admin_id=$2 AND request_id=$3`

func scanActionRequest(row pgx.Row) (actionRequest, error) {
	var req actionRequest
	var binding pgtype.Text
	var decidedAt pgtype.Timestamptz
	err := row.Scan(
		&req.RequestID, &req.OwnerAdminID, &req.Key, &req.Version, &req.fingerprint,
		&req.input, &req.Snapshot.RecordID, &req.Snapshot.Revision, &req.Snapshot.Fields, &binding,
		&req.CreatedAt, &req.ExpiresAt, &decidedAt, &req.State, &req.Result,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return actionRequest{}, errActionNotFound
	}
	req.BindingID, req.DecidedAt = binding.String, decidedAt.Time
	return req, err
}

func actionRequestID(raw string) (string, error) {
	id := strings.ToLower(strings.TrimSpace(raw))
	var uuid pgtype.UUID
	if len(id) != 36 || uuid.Scan(id) != nil || uuid.Bytes == [16]byte{} || uuid.String() != id {
		return "", errActionInvalid
	}
	return id, nil
}

func (s actionStore) propose(
	ctx context.Context, actor host.Actor, proposal actionProposal, authorize func(context.Context, ActionTx) error,
) (actionRequest, error) {
	if actor.AdminID <= 0 || authorize == nil {
		return actionRequest{}, errActionUnauthorized
	}
	id, err := actionRequestID(proposal.RequestID)
	if err != nil {
		return actionRequest{}, err
	}

	return s.run(ctx, func(ctx context.Context, tx pgx.Tx) (actionRequest, error) {
		// The key omits the action ID so concurrent proposals for distinct
		// actions still contend for the same UUID in this namespace.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", s.lockKey(actor.AdminID, id)); err != nil {
			return actionRequest{}, err
		}
		req, err := scanActionRequest(tx.QueryRow(ctx, selectActionRequest+" FOR UPDATE", s.action.Namespace, actor.AdminID, id))
		if err == nil {
			if err := authorize(ctx, tx); err != nil {
				return actionRequest{}, err
			}
			return s.replay(ctx, tx, req, proposal.Input)
		}
		if !errors.Is(err, errActionNotFound) {
			return actionRequest{}, err
		}
		return s.create(ctx, tx, actor, id, proposal, authorize)
	})
}

func (s actionStore) replay(
	ctx context.Context, tx pgx.Tx, req actionRequest, input map[string]json.RawMessage,
) (actionRequest, error) {
	if req.Key != s.action.Key {
		return actionRequest{}, errActionConflict
	}
	_, fingerprint, err := s.normalize(req.Version, input)
	if err != nil || !bytes.Equal(fingerprint[:], req.fingerprint) {
		return actionRequest{}, errActionConflict
	}
	if req.State != actionPending {
		return req, nil
	}
	now, err := actionClock(ctx, tx)
	if err != nil {
		return actionRequest{}, err
	}
	if !now.Before(req.ExpiresAt) {
		return s.finish(ctx, tx, req, actionExpired, nil)
	}
	return req, nil
}

func (s actionStore) create(
	ctx context.Context, tx pgx.Tx, actor host.Actor, id string, proposal actionProposal,
	authorize func(context.Context, ActionTx) error,
) (actionRequest, error) {
	if !validActionText(proposal.BindingID, 256) || proposal.ExpiresAt.IsZero() ||
		!validActionParameters(s.action.Parameters, proposal.Input, 0) {
		return actionRequest{}, errActionInvalid
	}
	input, fingerprint, err := s.normalize(s.action.Version, proposal.Input)
	if err != nil {
		return actionRequest{}, err
	}
	snapshot, inspectErr := s.action.Inspect(ctx, tx, actor, input, true)
	if err := authorize(ctx, tx); err != nil {
		return actionRequest{}, err
	}
	if actionRecordMissing(inspectErr) {
		return actionRequest{}, errActionNotFound
	}
	if inspectErr != nil {
		return actionRequest{}, inspectErr
	}
	if err := validateActionSnapshot(snapshot); err != nil {
		return actionRequest{}, err
	}
	now, err := actionClock(ctx, tx)
	if err != nil {
		return actionRequest{}, err
	}
	expires := proposal.ExpiresAt.Truncate(time.Microsecond)
	if !now.Before(expires) {
		return actionRequest{}, errActionExpired
	}
	expires = expires.UTC()
	if expires.After(now.Add(actionMaxLifetime)) {
		expires = now.Add(actionMaxLifetime)
	}
	return scanActionRequest(tx.QueryRow(ctx, `
		INSERT INTO admin_confirmed_actions
			(namespace, actor_admin_id, request_id, action_key, action_version, input_fingerprint,
			 normalized_input, record_id, record_revision, review_fields, binding_id, created_at,
			 expires_at, state)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, 'pending')
		RETURNING `+actionColumns,
		s.action.Namespace, actor.AdminID, id, s.action.Key, s.action.Version, fingerprint[:],
		input, snapshot.RecordID, snapshot.Revision, snapshot.Fields, proposal.BindingID, now, expires,
	))
}

func (s actionStore) status(ctx context.Context, actor host.Actor, requestID string) (actionRequest, error) {
	if actor.AdminID <= 0 {
		return actionRequest{}, errActionUnauthorized
	}
	id, err := actionRequestID(requestID)
	if err != nil {
		return actionRequest{}, err
	}
	return s.run(ctx, func(ctx context.Context, tx pgx.Tx) (actionRequest, error) {
		var ready bool
		query := "SELECT pg_try_advisory_xact_lock_shared(hashtextextended($1, 0))"
		if err := tx.QueryRow(ctx, query, s.lockKey(actor.AdminID, id)).Scan(&ready); err != nil {
			return actionRequest{}, err
		}
		if !ready {
			return actionRequest{}, errActionUnknown
		}
		req, err := scanActionRequest(tx.QueryRow(ctx, selectActionRequest+" FOR SHARE NOWAIT", s.action.Namespace, actor.AdminID, id))
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
		if req.State == actionPending {
			now, err := actionClock(ctx, tx)
			if err != nil {
				return actionRequest{}, err
			}
			if !now.Before(req.ExpiresAt) {
				req.State = actionExpired
				req.clearReview()
			}
		}
		return req, nil
	})
}

func (s actionStore) decide(
	ctx context.Context, actor host.Actor, requestID string, confirm bool,
	authorize func(context.Context, ActionTx, actionRequest) error,
) (actionRequest, error) {
	if actor.AdminID <= 0 || authorize == nil {
		return actionRequest{}, errActionUnauthorized
	}
	id, err := actionRequestID(requestID)
	if err != nil {
		return actionRequest{}, err
	}
	return s.run(ctx, func(ctx context.Context, tx pgx.Tx) (actionRequest, error) {
		req, err := scanActionRequest(tx.QueryRow(ctx, selectActionRequest+" FOR UPDATE", s.action.Namespace, actor.AdminID, id))
		if err != nil {
			return actionRequest{}, err
		}
		if req.Key != s.action.Key {
			return actionRequest{}, errActionNotFound
		}
		if req.State != actionPending {
			return req, nil
		}
		return s.decidePending(ctx, tx, actor, req, confirm, authorize)
	})
}

func (s actionStore) decidePending(
	ctx context.Context, tx pgx.Tx, actor host.Actor, req actionRequest, confirm bool,
	authorize func(context.Context, ActionTx, actionRequest) error,
) (actionRequest, error) {
	now, err := actionClock(ctx, tx)
	if err != nil {
		return actionRequest{}, err
	}
	var current ActionSnapshot
	var inspectErr error
	if now.Before(req.ExpiresAt) && confirm && req.Version == s.action.Version {
		current, inspectErr = s.action.Inspect(ctx, tx, actor, req.input, true)
	}
	authErr := authorize(ctx, tx, req)
	if authErr != nil && !errors.Is(authErr, errActionExpired) {
		return actionRequest{}, authErr
	}
	now, err = actionClock(ctx, tx)
	if err != nil {
		return actionRequest{}, err
	}
	switch {
	case !now.Before(req.ExpiresAt) || errors.Is(authErr, errActionExpired):
		return s.finish(ctx, tx, req, actionExpired, nil)
	case !confirm:
		return s.finish(ctx, tx, req, actionDeclined, nil)
	case inspectErr != nil && !actionRecordMissing(inspectErr):
		return actionRequest{}, inspectErr
	case req.Version != s.action.Version || inspectErr != nil ||
		req.Snapshot.RecordID != current.RecordID || req.Snapshot.Revision != current.Revision:
		return s.finish(ctx, tx, req, actionConflict, nil)
	}

	result, err := s.action.Apply(ctx, tx, actor, req.input, req.Snapshot, req.RequestID)
	if err != nil {
		return actionRequest{}, err
	}
	if err := s.validateResult(result); err != nil {
		return actionRequest{}, err
	}
	if result == nil {
		result = map[string]string{}
	}
	return s.finish(ctx, tx, req, actionSucceeded, result)
}

func (s actionStore) finish(
	ctx context.Context, tx ActionTx, req actionRequest, state string, result map[string]string,
) (actionRequest, error) {
	var resultValue any
	if result != nil {
		resultValue = result
	}
	err := tx.QueryRow(ctx, `
		UPDATE admin_confirmed_actions
		SET state=$4, result=$5, normalized_input=NULL, review_fields=NULL, binding_id=NULL, decided_at=clock_timestamp()
		WHERE namespace=$1 AND actor_admin_id=$2 AND request_id=$3 AND state='pending'
		RETURNING decided_at`,
		s.action.Namespace, req.OwnerAdminID, req.RequestID, state, resultValue,
	).Scan(&req.DecidedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return actionRequest{}, errActionUnknown
	}
	if err != nil {
		return actionRequest{}, err
	}
	req.State, req.Result = state, result
	req.clearReview()
	return req, nil
}

func (req *actionRequest) clearReview() {
	req.Snapshot.Fields = nil
	req.input = nil
	req.BindingID = ""
}

func (s actionStore) run(ctx context.Context, run func(context.Context, pgx.Tx) (actionRequest, error)) (actionRequest, error) {
	var req actionRequest
	callbackOK := false
	err := s.tx.RunInTx(ctx, func(ctx context.Context) error {
		tx := pgstorage.GetTx(ctx)
		if tx == nil {
			return errors.New("mcp: transaction manager did not provide a PostgreSQL transaction")
		}
		var err error
		req, err = run(ctx, tx)
		callbackOK = err == nil
		return err
	})
	if err != nil {
		// A broken connection at commit may mean the receipt is already durable.
		if callbackOK && !actionCommitDefinitelyFailed(err) {
			return actionRequest{}, fmt.Errorf("%w: %w", errActionUnknown, err)
		}
		return actionRequest{}, err
	}
	return req, nil
}

func actionCommitDefinitelyFailed(err error) bool {
	if errors.Is(err, pgx.ErrTxCommitRollback) {
		return true
	}
	var dbErr *pgconn.PgError
	return errors.As(err, &dbErr) && (strings.HasPrefix(dbErr.Code, "23") || dbErr.Code == "40001" || dbErr.Code == "40P01")
}

func actionClock(ctx context.Context, tx ActionTx) (time.Time, error) {
	var now time.Time
	err := tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now)
	return now, err
}

func (s actionStore) lockKey(owner int64, id string) string {
	return fmt.Sprintf("goadmin-confirmed-action/%s/%d/%s", s.action.Namespace, owner, id)
}

func (s actionStore) normalize(version string, raw map[string]json.RawMessage) (json.RawMessage, [sha256.Size]byte, error) {
	input, err := s.action.Normalize(version, raw)
	if err != nil || len(input) > 64<<10 || !utf8.Valid(input) || !json.Valid(input) {
		return nil, [sha256.Size]byte{}, errActionInvalid
	}
	parse := json.NewDecoder(bytes.NewReader(input))
	parse.UseNumber()
	var normalized map[string]any
	if err := parse.Decode(&normalized); err != nil || normalized == nil {
		return nil, [sha256.Size]byte{}, errActionInvalid
	}
	canonical, err := json.Marshal(normalized)
	if err != nil {
		return nil, [sha256.Size]byte{}, errActionInvalid
	}
	return canonical, sha256.Sum256(canonical), nil
}

func actionRecordMissing(err error) bool {
	return errors.Is(err, ErrActionNotFound) || errors.Is(err, crud.ErrNotFound)
}

func validActionText(value string, limit int) bool {
	return value != "" && len(value) <= limit && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

func validateActionSnapshot(snapshot ActionSnapshot) error {
	if !validActionText(snapshot.RecordID, 256) || !validActionText(snapshot.Revision, 256) ||
		len(snapshot.Fields) == 0 || len(snapshot.Fields) > 32 {
		return errors.New("mcp: invalid action snapshot")
	}
	total := 0
	for _, f := range snapshot.Fields {
		total += len(f.Label) + len(f.Value)
		if total > 1<<20 || !validActionText(f.Label, 200) || (f.Value != "" && !validActionText(f.Value, 256<<10)) {
			return errors.New("mcp: invalid action review field")
		}
	}
	return nil
}

// extra accounts for the UUID that the stdio input carries next to host parameters.
func validActionParameters(fields []Parameter, input map[string]json.RawMessage, extra int) bool {
	count := extra
	for _, field := range fields {
		if _, valid := field.parse(input[field.Name]); !valid {
			return false
		}
		if _, present := input[field.Name]; present {
			count++
		}
	}
	return count == len(input)
}

func (s actionStore) validateResult(result map[string]string) error {
	count := 0
	for _, field := range s.action.ResultFields {
		value, present := result[field.Key]
		if !present && field.Nullable {
			continue
		}
		if !present || (value != "" && !validActionText(value, 4000)) || (field.Kind == PositiveID && !validPositiveID(value)) {
			return errors.New("mcp: invalid action result field")
		}
		count++
	}
	if count != len(result) {
		return errors.New("mcp: undeclared action result field")
	}
	return nil
}
