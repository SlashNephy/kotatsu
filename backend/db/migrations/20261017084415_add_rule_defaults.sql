-- +goose Up
-- modify "system_settings" table
ALTER TABLE "system_settings" ADD COLUMN "rule_defaults" jsonb NOT NULL DEFAULT '{}';

-- +goose Down
-- reverse: modify "system_settings" table
ALTER TABLE "system_settings" DROP COLUMN "rule_defaults";
