-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS jobs_batches
(
    id             bigserial primary key,
    name           varchar(255) not null,
    total_jobs     integer      not null,
    pending_jobs   integer      not null,
    failed_jobs    integer      not null,
    failed_job_ids text         not null,
    options        text,
    cancelled_at   TIMESTAMPTZ,
    created_at     TIMESTAMPTZ  not null default now(),
    finished_at    TIMESTAMPTZ
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS jobs_batches;
-- +goose StatementEnd
