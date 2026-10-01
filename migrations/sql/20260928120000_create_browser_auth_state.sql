-- +goose Up
CREATE TABLE goadmin_browser_auth_state (
    id text PRIMARY KEY,
    version bigint NOT NULL CHECK (version > 0),
    owner text NOT NULL DEFAULT '',
    record jsonb NOT NULL,
    expires_at timestamptz NOT NULL
);
CREATE INDEX goadmin_browser_auth_state_expiry_idx ON goadmin_browser_auth_state(expires_at);

-- +goose Down
DROP TABLE goadmin_browser_auth_state;
