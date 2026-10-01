-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS admin_notifications
(
    id         BIGSERIAL PRIMARY KEY,
    admin_id   BIGINT       NOT NULL REFERENCES administrations (id) ON DELETE CASCADE,
    title      VARCHAR(255) NOT NULL,
    message    TEXT         NOT NULL,
    level      VARCHAR(32)  NOT NULL DEFAULT 'info',
    payload    JSONB        NOT NULL DEFAULT '{}'::jsonb,
    is_read    BOOLEAN      NOT NULL DEFAULT FALSE,
    read_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS admin_notifications_admin_id_idx ON admin_notifications (admin_id);
CREATE INDEX IF NOT EXISTS admin_notifications_unread_idx
    ON admin_notifications (admin_id)
    WHERE is_read = FALSE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS admin_notifications;
-- +goose StatementEnd
