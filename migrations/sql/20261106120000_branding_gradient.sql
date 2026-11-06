-- +goose Up
ALTER TABLE goadmin_branding ADD COLUMN accent_color_end VARCHAR(7) NOT NULL DEFAULT '';
-- +goose Down
ALTER TABLE goadmin_branding DROP COLUMN accent_color_end;
