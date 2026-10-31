// Package adminpreferences stores owner-scoped, versioned JSON documents.
package adminpreferences

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"

	"github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/internal/admintx"
)

const MaxPayloadBytes = 64 * 1024

var (
	ErrConflict      = errors.New("preferences changed; reload before saving")
	namespacePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,127}$`)
)

type Store struct{ db outbox.StoragePgsqlDBEngine }

func NewStore(db outbox.StoragePgsqlDBEngine) *Store { return &Store{db: db} }
func (s *Store) Load(ctx context.Context, owner int64, namespace string) (json.RawMessage, int64, error) {
	if s == nil ||
		s.db == nil {
		return nil, 0, errors.New("preferences database unavailable")
	}
	if owner <= 0 ||
		!namespacePattern.MatchString(namespace) {
		return nil, 0, errors.New("invalid preferences owner or namespace")
	}
	var row struct {
		Payload []byte `db:"payload"`
		Version int64  `db:"version"`
	}
	err := admintx.Wrap(s.db).ScanOne(ctx, "admin_preferences.load", &row,
		`SELECT payload,version FROM goadmin_admin_preferences WHERE admin_id=$1 AND namespace=$2`, owner, namespace)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("load preferences: %w", err)
	}
	return json.RawMessage(row.Payload), row.Version, nil
}

func (s *Store) Save(ctx context.Context, owner int64, namespace string, version int64, payload json.RawMessage) (int64, error) {
	if s == nil ||
		s.db == nil {
		return 0, errors.New("preferences database unavailable")
	}
	var object map[string]json.RawMessage
	if owner <= 0 ||
		version < 0 ||
		!namespacePattern.MatchString(namespace) ||
		len(payload) > MaxPayloadBytes ||
		json.Unmarshal(payload, &object) != nil ||
		object == nil {
		return 0, errors.New("invalid preferences document")
	}
	var next int64
	var err error
	if version == 0 {
		err = admintx.Wrap(s.db).ScanOne(ctx, "admin_preferences.create", &next,
			`INSERT INTO goadmin_admin_preferences(admin_id,namespace,payload) VALUES($1,$2,$3::jsonb)
ON CONFLICT(admin_id,namespace) DO NOTHING RETURNING version`, owner, namespace, string(payload))
	} else {
		err = admintx.Wrap(s.db).ScanOne(ctx, "admin_preferences.update", &next,
			`UPDATE goadmin_admin_preferences SET payload=$3::jsonb,version=version+1,updated_at=NOW()
WHERE admin_id=$1 AND namespace=$2 AND version=$4 RETURNING version`, owner, namespace, string(payload), version)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrConflict
	}
	if err != nil {
		return 0, fmt.Errorf("save preferences: %w", err)
	}
	return next, nil
}
