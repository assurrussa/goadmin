-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS admin_action_audit
(
    id                BIGSERIAL PRIMARY KEY,
    actor_admin_id    BIGINT      NOT NULL CHECK (actor_admin_id > 0),
    actor_subject_id  TEXT        NULL,
    action            TEXT        NOT NULL CHECK (length(action) BETWEEN 1 AND 100),
    target_type       TEXT        NOT NULL CHECK (length(target_type) BETWEEN 1 AND 100),
    target_id         TEXT        NOT NULL CHECK (length(target_id) BETWEEN 1 AND 200),
    related_target_id TEXT        NULL CHECK (related_target_id IS NULL OR length(related_target_id) <= 200),
    request_id        TEXT        NULL CHECK (request_id IS NULL OR length(request_id) <= 200),
    occurred_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS admin_action_audit_occurred_at_idx
    ON admin_action_audit (occurred_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS admin_action_audit_actor_idx
    ON admin_action_audit (actor_admin_id, occurred_at DESC, id DESC);

CREATE OR REPLACE FUNCTION reject_admin_action_audit_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'admin_action_audit is append-only';
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS admin_action_audit_no_row_mutation ON admin_action_audit;
CREATE TRIGGER admin_action_audit_no_row_mutation
    BEFORE UPDATE OR DELETE ON admin_action_audit
    FOR EACH ROW EXECUTE FUNCTION reject_admin_action_audit_mutation();
DROP TRIGGER IF EXISTS admin_action_audit_no_truncate ON admin_action_audit;
CREATE TRIGGER admin_action_audit_no_truncate
    BEFORE TRUNCATE ON admin_action_audit
    FOR EACH STATEMENT EXECUTE FUNCTION reject_admin_action_audit_mutation();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS admin_action_audit;
DROP FUNCTION IF EXISTS reject_admin_action_audit_mutation();
-- +goose StatementEnd
