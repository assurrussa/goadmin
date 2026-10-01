-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS goadmin_first_admin_setup_tokens
(
    id          BIGSERIAL PRIMARY KEY,
    token_hash  BYTEA                    NOT NULL,
    expires_at  TIMESTAMPTZ              NOT NULL,
    consumed_at TIMESTAMPTZ DEFAULT NULL,
    revoked_at  TIMESTAMPTZ DEFAULT NULL,
    created_at  TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    CONSTRAINT goadmin_first_admin_setup_tokens_hash_length CHECK (octet_length(token_hash) = 32)
);

CREATE UNIQUE INDEX IF NOT EXISTS goadmin_first_admin_setup_tokens_hash_idx
    ON goadmin_first_admin_setup_tokens (token_hash);

CREATE INDEX IF NOT EXISTS goadmin_first_admin_setup_tokens_active_idx
    ON goadmin_first_admin_setup_tokens (expires_at)
    WHERE consumed_at IS NULL AND revoked_at IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS goadmin_first_admin_setup_tokens;
-- +goose StatementEnd
