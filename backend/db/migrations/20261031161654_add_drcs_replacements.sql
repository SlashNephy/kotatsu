-- +goose Up
-- modify "system_settings" table
ALTER TABLE "system_settings" ADD COLUMN "drcs_replacements" jsonb NOT NULL DEFAULT '{}';

-- +goose Down
-- reverse: modify "system_settings" table
ALTER TABLE "system_settings" DROP COLUMN "drcs_replacements";
