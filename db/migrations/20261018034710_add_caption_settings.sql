-- +goose Up
-- modify "user_settings" table
ALTER TABLE "user_settings" ADD COLUMN "caption_settings" jsonb NULL;

-- +goose Down
-- reverse: modify "user_settings" table
ALTER TABLE "user_settings" DROP COLUMN "caption_settings";
