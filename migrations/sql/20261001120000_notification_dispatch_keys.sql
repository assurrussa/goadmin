-- +goose Up
-- +goose StatementBegin
ALTER TABLE admin_notifications ADD COLUMN IF NOT EXISTS dispatch_key text;
CREATE UNIQUE INDEX IF NOT EXISTS admin_notifications_dispatch_key_unique
    ON admin_notifications (admin_id, dispatch_key);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Forward-only: retry deduplication keys may belong to live jobs.
SELECT 1;
-- +goose StatementEnd
