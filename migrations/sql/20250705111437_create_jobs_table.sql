-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS jobs
(
    id           uuid primary key,
    queue        varchar(255) not null,
    name         varchar(255) not null,
    payload      text         not null,
    attempts     smallint     not null,
    reserved_at  TIMESTAMPTZ  null,
    available_at TIMESTAMPTZ  not null,
    created_at   TIMESTAMPTZ  not null DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS jobs_queue_index ON jobs (queue);
CREATE INDEX IF NOT EXISTS jobs_available_at_index ON jobs (available_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS jobs;
-- +goose StatementEnd
