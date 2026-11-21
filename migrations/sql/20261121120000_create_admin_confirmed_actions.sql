-- +goose Up
CREATE TABLE admin_confirmed_actions
(
    namespace          TEXT        NOT NULL CHECK (length(namespace) BETWEEN 1 AND 100),
    actor_admin_id     BIGINT      NOT NULL CHECK (actor_admin_id > 0),
    request_id         UUID        NOT NULL,
    action_key         TEXT        NOT NULL CHECK (length(action_key) BETWEEN 1 AND 100),
    action_version     TEXT        NOT NULL CHECK (length(action_version) BETWEEN 1 AND 100),
    input_fingerprint  BYTEA       NOT NULL CHECK (octet_length(input_fingerprint) = 32),
    normalized_input   JSONB,
    record_id          TEXT        NOT NULL,
    record_revision    TEXT        NOT NULL,
    review_fields      JSONB,
    binding_id         TEXT,
    created_at         TIMESTAMPTZ NOT NULL,
    expires_at         TIMESTAMPTZ NOT NULL CHECK (expires_at > created_at AND expires_at <= created_at + interval '15 minutes'),
    decided_at         TIMESTAMPTZ,
    state              TEXT        NOT NULL CHECK (state IN ('pending', 'succeeded', 'declined', 'expired', 'conflict')),
    result             JSONB,
    PRIMARY KEY (namespace, actor_admin_id, request_id),
    CHECK (
        (state = 'pending' AND normalized_input IS NOT NULL AND review_fields IS NOT NULL
            AND binding_id IS NOT NULL AND decided_at IS NULL)
        OR
        (state <> 'pending' AND normalized_input IS NULL AND review_fields IS NULL
            AND binding_id IS NULL)
    ),
    CHECK ((state = 'succeeded') = (result IS NOT NULL))
);

CREATE INDEX admin_confirmed_actions_pending_expiry_idx
    ON admin_confirmed_actions (namespace, action_key, expires_at, actor_admin_id, request_id)
    WHERE state = 'pending';

-- +goose Down
DROP TABLE admin_confirmed_actions;
