-- +goose Up
-- +goose StatementBegin
ALTER TABLE jobs
    ADD COLUMN IF NOT EXISTS schema_version integer NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS lease_token uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    ADD COLUMN IF NOT EXISTS deduplication_key text NULL;

ALTER TABLE jobs_failed
    ADD COLUMN IF NOT EXISTS schema_version integer NOT NULL DEFAULT 1;

CREATE INDEX IF NOT EXISTS jobs_capability_claim_index
    ON jobs (name, schema_version, available_at, reserved_at);

CREATE UNIQUE INDEX IF NOT EXISTS jobs_deduplication_key_unique
    ON jobs (deduplication_key)
    WHERE deduplication_key IS NOT NULL;

CREATE TABLE IF NOT EXISTS outbox_job_idempotency_keys
(
    deduplication_key text primary key,
    job_id            uuid        NOT NULL unique,
    fingerprint       text        NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS outbox_job_idempotency_keys_created_at_index
    ON outbox_job_idempotency_keys (created_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Forward-only: these columns and the idempotency registry are part of the
-- Outbox v0.12 runtime contract and may contain live job state.
SELECT 1;
-- +goose StatementEnd
