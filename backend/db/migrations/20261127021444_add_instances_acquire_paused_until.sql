-- +goose Up
-- modify "instances" table
ALTER TABLE "instances" ADD COLUMN "acquire_paused_until" timestamptz NULL;

-- +goose Down
-- reverse: modify "instances" table
ALTER TABLE "instances" DROP COLUMN "acquire_paused_until";
