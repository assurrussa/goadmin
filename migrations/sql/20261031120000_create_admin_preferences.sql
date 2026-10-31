-- +goose Up
CREATE TABLE goadmin_admin_preferences (
    admin_id BIGINT NOT NULL REFERENCES administrations(id) ON DELETE CASCADE,
    namespace VARCHAR(128) NOT NULL,
    payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object' AND octet_length(payload::text) <= 65536),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (admin_id, namespace)
);

-- +goose Down
DROP TABLE goadmin_admin_preferences;
