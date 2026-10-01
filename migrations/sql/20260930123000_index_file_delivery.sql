-- +goose Up
CREATE INDEX IF NOT EXISTS files_admin_delivery_path_idx
    ON files (manager_id, folder_path, filename) WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS files_admin_delivery_path_idx;
