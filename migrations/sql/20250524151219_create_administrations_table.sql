-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS administrations
(
    id         BIGSERIAL PRIMARY KEY,
    subject_id UUID                      NOT NULL REFERENCES auth_subjects (id) ON DELETE CASCADE,
    uuid       uuid                      NOT NULL,
    phone      bigint       DEFAULT NULL,
    version    int          DEFAULT 0,
    data       JSONB        DEFAULT NULL,
    created_at TIMESTAMPTZ  DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMPTZ  DEFAULT NOW() NOT NULL,
    deleted_at TIMESTAMPTZ  DEFAULT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS administrations_subject_id_idx ON administrations (subject_id);
CREATE UNIQUE INDEX IF NOT EXISTS administrations_uuid_idx ON administrations (uuid);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS administrations;
-- +goose StatementEnd
