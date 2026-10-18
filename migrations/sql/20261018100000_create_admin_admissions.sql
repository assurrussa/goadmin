-- +goose Up
-- +goose StatementBegin
CREATE TABLE admin_admissions (
 id UUID PRIMARY KEY,
 issuer TEXT NOT NULL CHECK (length(issuer) BETWEEN 1 AND 2048),
 project_id TEXT NOT NULL CHECK (length(project_id) BETWEEN 1 AND 128),
 client_id TEXT NOT NULL CHECK (length(client_id) BETWEEN 1 AND 128),
 external_subject TEXT NOT NULL CHECK (length(external_subject) BETWEEN 1 AND 512),
 subject_id UUID NOT NULL REFERENCES auth_subjects(id),
 status TEXT NOT NULL CHECK (status IN ('pending','approved','active','rejected')),
 version BIGINT NOT NULL CHECK (version > 0),
 approved_by UUID REFERENCES auth_subjects(id),
 grant_action TEXT NOT NULL DEFAULT '' CHECK (grant_action IN ('','invite','approve')),
 roles JSONB NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(roles) = 'array'),
 expires_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE (issuer, project_id, client_id, external_subject)
);
CREATE INDEX admin_admissions_status_idx ON admin_admissions(status, created_at, id);
CREATE TABLE admin_admission_invitations (
 id UUID PRIMARY KEY,
 issuer TEXT NOT NULL CHECK (length(issuer) BETWEEN 1 AND 2048),
 project_id TEXT NOT NULL CHECK (length(project_id) BETWEEN 1 AND 128),
 client_id TEXT NOT NULL CHECK (length(client_id) BETWEEN 1 AND 128),
 external_subject TEXT NOT NULL CHECK (length(external_subject) BETWEEN 1 AND 512),
 issued_by UUID NOT NULL REFERENCES auth_subjects(id),
 roles JSONB NOT NULL CHECK (jsonb_typeof(roles) = 'array'),
 expires_at TIMESTAMPTZ NOT NULL,
 revoked BOOLEAN NOT NULL DEFAULT false,
 accepted_subject UUID REFERENCES auth_subjects(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX admin_admission_invitations_subject_idx
 ON admin_admission_invitations(issuer, project_id, client_id, external_subject);
-- Pending subjects deliberately have no administrations row. Their enrollment
-- events cannot use actor_admin_id, which correctly requires a real admin.
CREATE TABLE admin_admission_events (
 id BIGSERIAL PRIMARY KEY,
 actor_subject_id UUID NOT NULL REFERENCES auth_subjects(id),
 action TEXT NOT NULL CHECK (length(action) BETWEEN 1 AND 100),
 target_id UUID NOT NULL,
 occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX admin_admission_events_target_idx ON admin_admission_events(target_id, id);
CREATE TRIGGER admin_admission_events_no_mutation BEFORE UPDATE OR DELETE
 ON admin_admission_events FOR EACH ROW EXECUTE FUNCTION reject_admin_action_audit_mutation();
CREATE TRIGGER admin_admission_events_no_truncate BEFORE TRUNCATE
 ON admin_admission_events FOR EACH STATEMENT EXECUTE FUNCTION reject_admin_action_audit_mutation();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE admin_admission_events;
DROP TABLE admin_admission_invitations;
DROP TABLE admin_admissions;
-- +goose StatementEnd
