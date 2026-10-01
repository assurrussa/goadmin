-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS users
(
    id                 BIGSERIAL PRIMARY KEY,
    subject_id         UUID         NOT NULL REFERENCES auth_subjects (id) ON DELETE CASCADE,
    uuid               uuid         NOT NULL,
    telegram_chat_id   BIGINT       NULL DEFAULT NULL,
    telegram_username  VARCHAR(255) NULL DEFAULT NULL,
    bio                VARCHAR(255) NULL DEFAULT NULL,
    phone              bigint       NULL DEFAULT NULL,
    version            int               DEFAULT 0,
    data               JSONB             DEFAULT NULL,
    confirmed_phone_at TIMESTAMPTZ       DEFAULT NULL,
    created_at         TIMESTAMPTZ       DEFAULT NOW() NOT NULL,
    updated_at         TIMESTAMPTZ       DEFAULT NOW() NOT NULL,
    deleted_at         TIMESTAMPTZ       DEFAULT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS users_subject_id_idx ON users (subject_id);
CREATE UNIQUE INDEX IF NOT EXISTS users_uuid_idx ON users (uuid);
CREATE UNIQUE INDEX IF NOT EXISTS users_phone_idx ON users (phone);
CREATE UNIQUE INDEX IF NOT EXISTS users_telegram_chat_id_idx ON users (telegram_chat_id);
CREATE UNIQUE INDEX IF NOT EXISTS users_telegram_username_idx ON users (telegram_username);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS users CASCADE;
-- +goose StatementEnd
