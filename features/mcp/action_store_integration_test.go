//go:build integration

package mcp //nolint:testpackage // Exercise the private persistence contract with PostgreSQL.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/assurrussa/outbox/backends/pgsql/storage/transaction"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/assurrussa/goadmin/host"
	"github.com/assurrussa/goadmin/tests"
)

var confirmedActor = host.Actor{AdminID: 101, SubjectID: "fixture-subject"}

func confirmedFixture(t *testing.T) (*pgxpool.Pool, actionStore) {
	t.Helper()
	db, _, cleanup := tests.PrepareDB(t.Context(), t, "ConfirmedActionStore")
	t.Cleanup(func() { cleanup(context.Background()) })
	pool := db.DB().Pool()
	_, err := pool.Exec(t.Context(), `
		CREATE TABLE confirmed_action_test_record (
			id text PRIMARY KEY, owner bigint NOT NULL, revision bigint NOT NULL DEFAULT 1
		);
		CREATE TABLE confirmed_action_test_event (
			request_id uuid PRIMARY KEY, record_id text NOT NULL
		);
		INSERT INTO confirmed_action_test_record (id, owner) VALUES ('r1', 101), ('r2', 101)
	`)
	require.NoError(t, err)
	action := Action{
		Namespace:    "test-namespace",
		Key:          "test-action",
		Version:      "v1",
		Parameters:   []Parameter{{Name: "record", Kind: Text, Required: true}},
		ResultFields: []Field{{Key: "event_id", Kind: Text}},
		Normalize: func(version string, in map[string]json.RawMessage) (json.RawMessage, error) {
			if version != "v1" && version != "v2" {
				return nil, errors.New("unsupported version")
			}
			return json.Marshal(in)
		},
		Inspect: func(ctx context.Context, tx ActionTx, actor host.Actor, input json.RawMessage, lock bool) (ActionSnapshot, error) {
			var params struct {
				Record string `json:"record"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return ActionSnapshot{}, err
			}
			query := "SELECT revision::text FROM confirmed_action_test_record WHERE id=$1 AND owner=$2"
			if lock {
				query += " FOR UPDATE"
			}
			snapshot := ActionSnapshot{RecordID: params.Record, Fields: []ReviewField{{Label: "Private summary", Value: params.Record}}}
			err := tx.QueryRow(ctx, query, params.Record, actor.AdminID).Scan(&snapshot.Revision)
			if errors.Is(err, pgx.ErrNoRows) {
				return ActionSnapshot{}, ErrActionNotFound
			}
			return snapshot, err
		},
		Apply: func(
			ctx context.Context, tx ActionTx, _ host.Actor, _ json.RawMessage, snapshot ActionSnapshot, id string,
		) (map[string]string, error) {
			const update = "UPDATE confirmed_action_test_record SET revision=revision+1 WHERE id=$1"
			if _, err := tx.Exec(ctx, update, snapshot.RecordID); err != nil {
				return nil, err
			}
			if _, err := tx.Exec(ctx, "INSERT INTO confirmed_action_test_event VALUES ($1,$2)", id, snapshot.RecordID); err != nil {
				return nil, err
			}
			return map[string]string{"event_id": id}, nil
		},
	}
	return pool, actionStore{tx: host.NewTxManager(db), action: action}
}

func confirmedProposal(t *testing.T, record string) actionProposal {
	t.Helper()
	raw, err := json.Marshal(record)
	require.NoError(t, err)
	return actionProposal{
		RequestID: uuid.NewString(),
		Input:     map[string]json.RawMessage{"record": raw},
		BindingID: "issuing-grant",
		ExpiresAt: time.Now().Add(time.Minute),
	}
}

func confirmedAuthorize(ctx context.Context, tx ActionTx) error {
	// This query is deliberately made through the passed transaction. It also
	// proves that authorizing a proposal works with a single pooled connection.
	var value int
	return tx.QueryRow(ctx, "SELECT 1").Scan(&value)
}

func confirmedAuthorizeDecision(ctx context.Context, tx ActionTx, req actionRequest) error {
	if req.BindingID != "issuing-grant" {
		return errActionNotFound
	}
	return confirmedAuthorize(ctx, tx)
}

func TestConfirmedActionStoreRollbackAndSingleConnection(t *testing.T) {
	pool, store := confirmedFixture(t)
	config := pool.Config()
	config.MinConns = 0
	config.MaxConns = 1
	single, err := pgxpool.NewWithConfig(t.Context(), config)
	require.NoError(t, err)
	t.Cleanup(single.Close)
	store.tx = transaction.New(single)

	proposal := confirmedProposal(t, "r1")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	req, err := store.propose(ctx, confirmedActor, proposal, confirmedAuthorize)
	require.NoError(t, err)
	require.Equal(t, actionPending, req.State)
	require.NotEmpty(t, req.Snapshot.Fields)

	originalApply := store.action.Apply
	failed := errors.New("audit write failed")
	store.action.Apply = func(
		ctx context.Context, tx ActionTx, actor host.Actor, input json.RawMessage, snapshot ActionSnapshot, id string,
	) (map[string]string, error) {
		_, err := originalApply(ctx, tx, actor, input, snapshot, id)
		if err != nil {
			return nil, err
		}
		return nil, failed
	}
	_, err = store.decide(ctx, confirmedActor, proposal.RequestID, true, confirmedAuthorizeDecision)
	require.ErrorIs(t, err, failed)
	req, err = store.status(ctx, confirmedActor, proposal.RequestID)
	require.NoError(t, err)
	require.Equal(t, actionPending, req.State)
	require.Equal(t, "r1", req.Snapshot.RecordID)
	var revision, events int
	const effects = `SELECT revision, (SELECT count(*) FROM confirmed_action_test_event)
	    FROM confirmed_action_test_record WHERE id='r1'`
	require.NoError(t, single.QueryRow(ctx, effects).Scan(&revision, &events))
	require.Equal(t, 1, revision)
	require.Zero(t, events)

	store.action.Apply = originalApply
	req, err = store.decide(ctx, confirmedActor, proposal.RequestID, true, confirmedAuthorizeDecision)
	require.NoError(t, err)
	require.Equal(t, actionSucceeded, req.State)
	require.Equal(t, "r1", req.Snapshot.RecordID)
	require.Equal(t, "1", req.Snapshot.Revision)
	require.False(t, req.DecidedAt.IsZero())
	require.Equal(t, map[string]string{"event_id": proposal.RequestID}, req.Result)
	require.Empty(t, req.Snapshot.Fields)
	require.Empty(t, req.BindingID)
	require.Empty(t, req.input)
	require.NoError(t, single.QueryRow(ctx, effects).Scan(&revision, &events))
	require.Equal(t, 2, revision)
	require.Equal(t, 1, events)

	terminal, err := store.decide(ctx, confirmedActor, proposal.RequestID, false,
		func(context.Context, ActionTx, actionRequest) error {
			return errors.New("terminal replay invoked authorization")
		})
	require.NoError(t, err)
	require.Equal(t, req, terminal)
}

func TestConfirmedActionStoreUUIDAndOriginalBinding(t *testing.T) {
	pool, store := confirmedFixture(t)
	proposal := confirmedProposal(t, "r1")
	original, err := store.propose(t.Context(), confirmedActor, proposal, confirmedAuthorize)
	require.NoError(t, err)

	upgraded := store
	upgraded.action.Version = "v2"
	upgraded.action.Normalize = func(version string, input map[string]json.RawMessage) (json.RawMessage, error) {
		if version != "v1" {
			return nil, fmt.Errorf("replay normalized using %s", version)
		}
		return store.action.Normalize(version, input)
	}
	replay := proposal
	replay.BindingID = "later-grant"
	replay.ExpiresAt = proposal.ExpiresAt.Add(time.Minute)
	req, err := upgraded.propose(t.Context(), confirmedActor, replay, confirmedAuthorize)
	require.NoError(t, err)
	require.Equal(t, original, req)

	other := store
	other.action.Key = "other-action"
	_, err = other.propose(t.Context(), confirmedActor, replay, confirmedAuthorize)
	require.ErrorIs(t, err, errActionConflict)
	_, err = other.status(t.Context(), confirmedActor, proposal.RequestID)
	require.ErrorIs(t, err, errActionNotFound)
	_, err = store.status(t.Context(), host.Actor{AdminID: 102}, proposal.RequestID)
	require.ErrorIs(t, err, errActionNotFound)
	changed := replay
	changed.Input = confirmedProposal(t, "r2").Input
	_, err = upgraded.propose(t.Context(), confirmedActor, changed, confirmedAuthorize)
	require.ErrorIs(t, err, errActionConflict)

	_, err = store.decide(t.Context(), confirmedActor, proposal.RequestID, true,
		func(context.Context, ActionTx, actionRequest) error { return errActionNotFound })
	require.ErrorIs(t, err, errActionNotFound)
	req, err = store.status(t.Context(), confirmedActor, proposal.RequestID)
	require.NoError(t, err)
	require.Equal(t, original, req)
	req, err = upgraded.decide(t.Context(), confirmedActor, proposal.RequestID, true, confirmedAuthorizeDecision)
	require.NoError(t, err)
	require.Equal(t, actionConflict, req.State)
	require.Empty(t, req.Snapshot.Fields)

	// The legacy pilot did not record the time for every disposition.
	_, err = pool.Exec(t.Context(), "UPDATE admin_confirmed_actions SET decided_at=NULL WHERE request_id=$1", proposal.RequestID)
	require.NoError(t, err)
	imported, err := upgraded.propose(t.Context(), confirmedActor, replay, confirmedAuthorize)
	require.NoError(t, err)
	require.Equal(t, actionConflict, imported.State)
	require.Zero(t, imported.DecidedAt)
	require.Equal(t, req.Snapshot, imported.Snapshot)
}

func TestConfirmedActionStoreReportsInFlightAndAppliesOnce(t *testing.T) {
	pool, store := confirmedFixture(t)
	proposal := confirmedProposal(t, "r1")
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	type outcome struct {
		req actionRequest
		err error
	}
	entered, release := make(chan struct{}), make(chan struct{})
	proposed := make(chan outcome, 1)
	go func() {
		req, err := store.propose(ctx, confirmedActor, proposal, func(ctx context.Context, _ ActionTx) error {
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		proposed <- outcome{req, err}
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	_, err := store.status(ctx, confirmedActor, proposal.RequestID)
	close(release)
	require.ErrorIs(t, err, errActionUnknown)
	require.NoError(t, (<-proposed).err)

	entered, release = make(chan struct{}), make(chan struct{})
	decided := make(chan outcome, 1)
	go func() {
		authorize := func(ctx context.Context, tx ActionTx, req actionRequest) error {
			close(entered)
			select {
			case <-release:
				return confirmedAuthorizeDecision(ctx, tx, req)
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		req, err := store.decide(ctx, confirmedActor, proposal.RequestID, true, authorize)
		decided <- outcome{req, err}
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	replayed := make(chan outcome, 1)
	go func() {
		req, err := store.decide(ctx, confirmedActor, proposal.RequestID, true, confirmedAuthorizeDecision)
		replayed <- outcome{req, err}
	}()
	_, err = store.status(ctx, confirmedActor, proposal.RequestID)
	_, expireErr := store.expire(ctx, confirmedActor, proposal.RequestID)
	close(release)
	require.ErrorIs(t, err, errActionUnknown)
	require.ErrorIs(t, expireErr, errActionUnknown)
	first, repeat := <-decided, <-replayed
	require.NoError(t, first.err)
	require.NoError(t, repeat.err)
	require.Equal(t, actionSucceeded, first.req.State)
	require.Equal(t, first.req, repeat.req)
	var events int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM confirmed_action_test_event").Scan(&events))
	require.Equal(t, 1, events)
}

func TestConfirmedActionStoreRechecksExpiryAfterWaitingForRecord(t *testing.T) {
	pool, store := confirmedFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	proposal := confirmedProposal(t, "r1")
	_, err := store.propose(ctx, confirmedActor, proposal, confirmedAuthorize)
	require.NoError(t, err)

	var expires time.Time
	require.NoError(t, pool.QueryRow(ctx, `
		UPDATE admin_confirmed_actions SET expires_at=clock_timestamp()+interval '2 seconds'
		WHERE request_id=$1 RETURNING expires_at`, proposal.RequestID).Scan(&expires))
	lock, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer lock.Rollback(context.WithoutCancel(ctx))
	_, err = lock.Exec(ctx, "SELECT 1 FROM confirmed_action_test_record WHERE id='r1' FOR UPDATE")
	require.NoError(t, err)

	inspecting := make(chan struct{})
	inspect := store.action.Inspect
	store.action.Inspect = func(
		ctx context.Context, tx ActionTx, actor host.Actor, in json.RawMessage, takeLock bool,
	) (ActionSnapshot, error) {
		close(inspecting)
		return inspect(ctx, tx, actor, in, takeLock)
	}
	type outcome struct {
		req actionRequest
		err error
	}
	decided := make(chan outcome, 1)
	go func() {
		req, err := store.decide(ctx, confirmedActor, proposal.RequestID, true, confirmedAuthorizeDecision)
		decided <- outcome{req, err}
	}()
	select {
	case <-inspecting:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	_, sleepErr := lock.Exec(ctx, "SELECT pg_sleep_until($1)", expires)
	unlockErr := lock.Commit(ctx)
	require.NoError(t, sleepErr)
	require.NoError(t, unlockErr)
	result := <-decided
	require.NoError(t, result.err)
	require.Equal(t, actionExpired, result.req.State)
	require.Empty(t, result.req.Snapshot.Fields)
	require.False(t, result.req.DecidedAt.Before(expires))
	var events int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM confirmed_action_test_event").Scan(&events))
	require.Zero(t, events)
}

type lostCommitReply struct{ host.StoragePgsqlTxManager }

func (tx lostCommitReply) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	if err := tx.StoragePgsqlTxManager.RunInTx(ctx, fn); err != nil {
		return err
	}
	return errors.New("connection closed after COMMIT")
}

func TestConfirmedActionStoreReconcilesLostCommitReply(t *testing.T) {
	pool, store := confirmedFixture(t)
	ctx := t.Context()
	proposal := confirmedProposal(t, "r1")
	_, err := store.propose(ctx, confirmedActor, proposal, confirmedAuthorize)
	require.NoError(t, err)
	disconnected := store
	disconnected.tx = lostCommitReply{store.tx}
	_, err = disconnected.decide(ctx, confirmedActor, proposal.RequestID, true, confirmedAuthorizeDecision)
	require.ErrorIs(t, err, errActionUnknown)
	receipt, err := store.status(ctx, confirmedActor, proposal.RequestID)
	require.NoError(t, err)
	require.Equal(t, actionSucceeded, receipt.State)
	require.Equal(t, proposal.RequestID, receipt.Result["event_id"])
	replay, err := store.propose(ctx, confirmedActor, proposal, confirmedAuthorize)
	require.NoError(t, err)
	require.Equal(t, receipt, replay)
	var events int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM confirmed_action_test_event").Scan(&events))
	require.Equal(t, 1, events)
}

func TestConfirmedActionStoreDeadlineDuringApplyRollsBack(t *testing.T) {
	pool, store := confirmedFixture(t)
	ctx := t.Context()
	p := confirmedProposal(t, "r1")
	p.ExpiresAt = time.Now().Add(time.Second)
	_, err := store.propose(ctx, confirmedActor, p, confirmedAuthorize)
	require.NoError(t, err)
	apply := store.action.Apply
	applied := false
	store.action.Apply = func(
		ctx context.Context, tx ActionTx, actor host.Actor, input json.RawMessage, snapshot ActionSnapshot, id string,
	) (map[string]string, error) {
		result, err := apply(ctx, tx, actor, input, snapshot, id)
		if err != nil {
			return nil, err
		}
		applied = true
		_, err = tx.Exec(ctx, `SELECT pg_sleep(greatest(0,extract(epoch FROM expires_at-clock_timestamp()))::float8)
			FROM admin_confirmed_actions WHERE request_id=$1`, id)
		return result, err
	}
	receipt, err := store.decide(ctx, confirmedActor, p.RequestID, true, confirmedAuthorizeDecision)
	require.NoError(t, err)
	require.True(t, applied)
	require.Equal(t, actionExpired, receipt.State)
	var revision, events int
	require.NoError(t, pool.QueryRow(ctx, `SELECT revision,(SELECT count(*) FROM confirmed_action_test_event)
		FROM confirmed_action_test_record WHERE id='r1'`).Scan(&revision, &events))
	require.Equal(t, 1, revision)
	require.Zero(t, events)
}
