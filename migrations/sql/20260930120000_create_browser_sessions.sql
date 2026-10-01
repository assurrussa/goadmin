-- +goose Up
CREATE TABLE goadmin_browser_sessions (
    id text PRIMARY KEY,
    payload bytea NOT NULL,
    expires_at timestamptz
);
CREATE INDEX goadmin_browser_sessions_expiry_idx ON goadmin_browser_sessions(expires_at)
    WHERE expires_at IS NOT NULL;

-- +goose Down
DROP TABLE goadmin_browser_sessions;
