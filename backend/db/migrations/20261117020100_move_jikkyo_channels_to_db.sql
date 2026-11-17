-- +goose Up
-- modify "system_settings" table
ALTER TABLE "system_settings" ADD COLUMN "jikkyo_channels" jsonb NOT NULL DEFAULT '{}';

-- +goose Down
-- reverse: modify "system_settings" table
ALTER TABLE "system_settings" DROP COLUMN "jikkyo_channels";
