-- +goose Up
-- modify "system_settings" table
ALTER TABLE "system_settings" ADD COLUMN "jikkyo_enabled" boolean NOT NULL DEFAULT false;
-- modify "user_settings" table
ALTER TABLE "user_settings" ALTER COLUMN "jikkyo_enabled" SET DEFAULT false;

-- +goose Down
-- reverse: modify "user_settings" table
ALTER TABLE "user_settings" ALTER COLUMN "jikkyo_enabled" SET DEFAULT true;
-- reverse: modify "system_settings" table
ALTER TABLE "system_settings" DROP COLUMN "jikkyo_enabled";
