package browserstate

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/assurrussa/goauth"
)

type Postgres struct {
	db    *sql.DB
	codec codec
}

func NewPostgres(db *sql.DB, keys goauth.KeyRing) *Postgres {
	return &Postgres{db: db, codec: codec{keys: keys}}
}

func (s *Postgres) Load(ctx context.Context, id string) (Record, bool, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT record FROM goadmin_browser_auth_state WHERE id=$1 AND expires_at>now()`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, err
	}
	r, err := s.codec.decode(id, raw)
	return r, err == nil, err
}

func (s *Postgres) Create(ctx context.Context, id string, r Record, ttl time.Duration) error {
	if ttl <= 0 {
		return errors.New("admin auth state lifetime must be positive")
	}
	r.Version = 1
	r.Owner = ""
	raw, err := s.codec.encode(id, r)
	if err != nil {
		return err
	}
	// Opportunistic bounded cleanup; backups/WAL still need normal retention.
	if _, err := s.db.ExecContext(ctx, `DELETE FROM goadmin_browser_auth_state WHERE id IN
 (SELECT id FROM goadmin_browser_auth_state WHERE expires_at<=now() LIMIT 500)`); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO goadmin_browser_auth_state(id,version,owner,record,expires_at)
 VALUES($1,1,'',$2,$3)`, id, raw, time.Now().UTC().Add(ttl))
	return err
}

func (s *Postgres) Claim(ctx context.Context, id string, v int64, owner string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE goadmin_browser_auth_state SET owner=$3,
 record=jsonb_set(record,'{owner}',to_jsonb($3::text))
 WHERE id=$1 AND version=$2 AND owner='' AND expires_at>now()`, id, v, owner)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func (s *Postgres) Complete(ctx context.Context, id string, v int64, owner string, r Record) (bool, error) {
	r.Version = v + 1
	r.Owner = ""
	raw, err := s.codec.encode(id, r)
	if err != nil {
		return false, err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE goadmin_browser_auth_state SET version=$2+1,owner='',record=$4
 WHERE id=$1 AND version=$2 AND owner=$3 AND expires_at>now()`, id, v, owner, raw)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func (s *Postgres) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM goadmin_browser_auth_state WHERE id=$1`, id)
	return err
}
