-- +goose Up
-- +goose StatementBegin
CREATE TABLE admin_profile_presentations (
    subject_id UUID PRIMARY KEY REFERENCES auth_subjects(id) ON DELETE CASCADE,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    use_name BOOLEAN NOT NULL DEFAULT false,
    use_login BOOLEAN NOT NULL DEFAULT false,
    use_avatar BOOLEAN NOT NULL DEFAULT false,
    issuer TEXT NOT NULL DEFAULT '',
    external_subject TEXT NOT NULL DEFAULT '',
    link_id TEXT NOT NULL DEFAULT '',
    client_id TEXT NOT NULL DEFAULT '',
    project_id TEXT NOT NULL DEFAULT '',
    display_name VARCHAR(256) NOT NULL DEFAULT '',
    display_login VARCHAR(256) NOT NULL DEFAULT '',
    avatar_url VARCHAR(256) NOT NULL DEFAULT '',
    issued_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    pending JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(pending) = 'object' AND octet_length(pending::text) <= 8192)
);
CREATE UNIQUE INDEX admin_profile_refresh_selector
    ON admin_profile_presentations ((pending->>'selector')) WHERE pending ? 'selector';
-- +goose StatementEnd

-- +goose Down
DROP TABLE admin_profile_presentations;
