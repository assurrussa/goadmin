package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/assurrussa/goadmin/host"
)

// ErrActionNotFound indicates that the record is absent or ineligible for
// the declared action. Inspect should use it without distinguishing the cases.
var ErrActionNotFound = errors.New("mcp: eligible action record not found")

// Action declares one versioned action that an operator must approve in the
// browser. Namespace and Key identify the action, and Version identifies its
// normalization and execution contract.
type Action struct {
	Namespace    string
	Key          string
	Version      string
	Path         string
	Title        string
	Description  string
	ConfirmLabel string
	ProposeTool  string
	StatusTool   string
	Permissions  []host.PermissionKey
	Parameters   []Parameter
	ResultFields []Field
	// PreviousResultFields declares earlier receipt schemas when a version
	// used different result fields. Versions with an unchanged schema need no entry.
	PreviousResultFields map[string][]Field
	ReceiptLink          *ReceiptLink

	// Requires lists other host modules that must be installed with this action.
	Requires []string

	// Normalize is deterministic and supports stored versions whose requests
	// may be retried. It returns the JSON object to persist until disposition.
	Normalize func(version string, input map[string]json.RawMessage) (json.RawMessage, error)
	// Inspect reads the record and revision that the browser reviews, scoped
	// to the actor. When lock is true it must lock the record and any other
	// data whose change would invalidate the proposed action.
	Inspect func(ctx context.Context, tx ActionTx, actor host.Actor, input json.RawMessage, lock bool) (ActionSnapshot, error)
	// LoadReview optionally supplies the full browser-only review instead of
	// persisting Inspect.Fields. Return the fields and the identity and revision
	// they describe from a consistent read, without locking the domain record.
	// GoAdmin displays the fields only when both match the pending proposal.
	LoadReview func(ctx context.Context, tx ActionTx, actor host.Actor, input json.RawMessage) (ActionSnapshot, error)
	// Apply uses the locked, rechecked snapshot and writes all domain changes
	// and audit data through tx. The result may contain only declared fields.
	Apply func(
		ctx context.Context, tx ActionTx, actor host.Actor, input json.RawMessage, snapshot ActionSnapshot, requestID string,
	) (map[string]string, error)
}

// ReceiptLink is an optional browser-only link to an installed, permission-guarded
// host page such as the product journal. Href must be a local route.
type ReceiptLink struct {
	Href       string
	Label      string
	Permission host.PermissionKey
}

// ActionTx provides access to the action's transaction without ownership of
// commit or rollback.
type ActionTx interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// ActionSnapshot represents the eligible record at a revision. Fields are
// plain text for the browser review and are discarded at disposition.
type ActionSnapshot struct {
	RecordID string
	Revision string
	Fields   []ReviewField
}

// ReviewField labels one plain-text value in the operator's review.
type ReviewField struct {
	Label string `json:"label"`
	Value string `json:"value"`
}
