-- +goose Up
CREATE TABLE goadmin_branding (
 singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
 version BIGINT NOT NULL DEFAULT 0 CHECK (version >= 0),
 name VARCHAR(80) NOT NULL DEFAULT '',
 accent_color VARCHAR(7) NOT NULL DEFAULT '',
 logo_light BYTEA NOT NULL DEFAULT ''::bytea,
 logo_dark BYTEA NOT NULL DEFAULT ''::bytea,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO goadmin_branding(singleton) VALUES(TRUE);
-- +goose Down
DROP TABLE goadmin_branding;
