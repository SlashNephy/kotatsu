-- +goose Up
-- modify "user_settings" table
ALTER TABLE "user_settings" ADD COLUMN "preferences" jsonb NOT NULL DEFAULT '{}';

-- +goose Down
-- reverse: modify "user_settings" table
ALTER TABLE "user_settings" DROP COLUMN "preferences";
