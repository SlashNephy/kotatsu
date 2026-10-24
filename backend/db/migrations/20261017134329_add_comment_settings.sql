-- +goose Up
-- modify "user_settings" table
ALTER TABLE "user_settings" ADD COLUMN "comment_settings" jsonb NULL;

-- +goose Down
-- reverse: modify "user_settings" table
ALTER TABLE "user_settings" DROP COLUMN "comment_settings";
