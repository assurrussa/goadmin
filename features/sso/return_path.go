package sso

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"time"

	browser "github.com/assurrussa/gossoclient"
)

func returnSchema() string {
	return `CREATE TABLE IF NOT EXISTS sso_return_paths (
 authority_id text NOT NULL,
 login_generation bigint NOT NULL,
 target text NOT NULL,
 expires_at timestamptz NOT NULL,
 consumed_at timestamptz,
 PRIMARY KEY (authority_id, login_generation)
);
CREATE TABLE IF NOT EXISTS sso_login_return_intents (
 state_digest text PRIMARY KEY,
 target text NOT NULL,
 expires_at timestamptz NOT NULL,
 consumed_at timestamptz
);
CREATE INDEX IF NOT EXISTS sso_return_paths_expiry ON sso_return_paths(expires_at);
CREATE INDEX IF NOT EXISTS sso_login_return_intents_expiry ON sso_login_return_intents(expires_at);`
}

func safeReturnPath(raw, origin string) string {
	if len(raw) > 512 {
		return "/"
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil {
		return "/"
	}
	if u.IsAbs() || u.Host != "" {
		if u.Scheme+"://"+u.Host != origin {
			return "/"
		}
		u.Scheme, u.Host = "", ""
	}
	target := u.String()
	decoded := target
	for i := 0; i < 6; i++ {
		if !strings.HasPrefix(decoded, "/") || strings.HasPrefix(decoded, "//") || strings.ContainsAny(decoded, "\\\r\n\t\x00") {
			return "/"
		}
		for _, r := range decoded {
			if r < 32 || r == 127 {
				return "/"
			}
		}
		next, e := url.PathUnescape(decoded)
		if e != nil {
			return "/"
		}
		if next == decoded {
			break
		}
		decoded = next
		if i == 5 {
			return "/"
		}
	}
	parsed, err := url.Parse(decoded)
	if err != nil {
		return "/"
	}
	switch parsed.Path {
	case "/login", "/logout", callbackPath, "/sso", "/sso/open", "/sso-link", "/auth/login", "/auth/session/recover":
		return "/"
	}
	return target
}

func (s *Runtime) saveReturn(ctx context.Context, b browser.Binding, target string) error {
	//nolint:gosec // Schema is validated as a restricted identifier before runtime construction.
	_, err := s.db.ExecContext(ctx, `INSERT INTO `+s.tablePrefix()+`sso_return_paths
 (authority_id,login_generation,target,expires_at) VALUES ($1,$2,$3,$4)
 ON CONFLICT (authority_id,login_generation) DO NOTHING`,
		b.Reference, b.LoginGeneration, safeReturnPath(target, s.Config.Origin), time.Now().Add(5*time.Minute))
	return err
}

func (s *Runtime) takeReturn(ctx context.Context, b browser.Binding) (string, error) {
	var target string
	err := s.db.QueryRowContext(ctx, `UPDATE `+s.tablePrefix()+`sso_return_paths SET consumed_at=clock_timestamp()
 WHERE authority_id=$1 AND login_generation=$2 AND consumed_at IS NULL AND expires_at>clock_timestamp()
 RETURNING target`, b.Reference, b.LoginGeneration).Scan(&target)
	if err == sql.ErrNoRows {
		return "/", nil
	}
	if err != nil {
		return "", err
	}
	return safeReturnPath(target, s.Config.Origin), nil
}

func stateDigest(state string) string {
	hash := sha256.Sum256([]byte(state))
	return hex.EncodeToString(hash[:])
}

func (s *Runtime) saveLoginReturn(ctx context.Context, state, target string) error {
	if err := s.pruneNavigation(ctx); err != nil {
		return err
	}
	if state == "" || len(state) > 1024 {
		return ErrDenied
	}
	//nolint:gosec // Schema is validated as a restricted identifier before runtime construction.
	_, err := s.db.ExecContext(ctx, `INSERT INTO `+s.tablePrefix()+`sso_login_return_intents
 (state_digest,target,expires_at) VALUES ($1,$2,clock_timestamp()+interval '5 minutes')`,
		stateDigest(state), safeReturnPath(target, s.Config.Origin))
	return err
}

func (s *Runtime) takeLoginReturn(ctx context.Context, state string) (string, error) {
	if state == "" || len(state) > 1024 {
		return "", ErrDenied
	}
	var target string
	err := s.db.QueryRowContext(ctx, `UPDATE `+s.tablePrefix()+`sso_login_return_intents SET consumed_at=clock_timestamp()
 WHERE state_digest=$1 AND consumed_at IS NULL AND expires_at>clock_timestamp() RETURNING target`,
		stateDigest(state)).Scan(&target)
	if err == sql.ErrNoRows {
		return "/", nil
	}
	if err != nil {
		return "", err
	}
	return safeReturnPath(target, s.Config.Origin), nil
}

func (s *Runtime) tablePrefix() string { return `"` + s.Config.schema() + `".` }

// peekReturn never consumes navigation before canonical admission succeeds.
func (s *Runtime) peekReturn(ctx context.Context, b browser.Binding) (string, error) {
	var target string
	err := s.db.QueryRowContext(ctx, `SELECT target FROM `+s.tablePrefix()+`sso_return_paths
 WHERE authority_id=$1 AND login_generation=$2 AND consumed_at IS NULL AND expires_at>clock_timestamp()`,
		b.Reference, b.LoginGeneration).Scan(&target)
	if errors.Is(err, sql.ErrNoRows) {
		return "/", nil
	}
	if err != nil {
		return "", err
	}
	return safeReturnPath(target, s.Config.Origin), nil
}

func (s *Runtime) pruneNavigation(ctx context.Context) error {
	for _, entry := range []struct{ table, keys string }{
		{"sso_return_paths", "authority_id,login_generation"}, {"sso_login_return_intents", "state_digest"},
	} {
		if err := pruneExpired(ctx, s.db, s.tablePrefix()+entry.table, entry.keys); err != nil {
			return err
		}
	}
	return nil
}

// Identifiers are internal constants plus the validated private schema/table name.
// Expired markers cannot authorize a replay; unexpired markers are preserved.
func pruneExpired(ctx context.Context, db *sql.DB, table, keys string) error {
	//nolint:gosec // Only validated schema names and internal constant identifiers enter this query.
	_, err := db.ExecContext(ctx, `DELETE FROM `+table+` WHERE (`+keys+`) IN (
 SELECT `+keys+` FROM `+table+` WHERE expires_at <= clock_timestamp()
 ORDER BY expires_at LIMIT 128 FOR UPDATE SKIP LOCKED)`)
	return err
}
