-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS jobs_failed
(
    id         uuid primary key,
    job_id     uuid                      not null
        constraint jobs_failed_uuid_unique unique,
    connection text                      not null,
    queue      text                      not null,
    name       text                      not null,
    payload    text                      not null,
    reason     text                      not null,
    exception  text                      not null,
    failed_at  TIMESTAMPTZ default now() not null,
    created_at TIMESTAMPTZ default now() not null
);

CREATE INDEX IF NOT EXISTS jobs_failed_queue_index ON jobs_failed (queue);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS jobs_failed;
-- +goose StatementEnd
