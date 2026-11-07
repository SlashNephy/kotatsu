-- +goose Up
-- modify "tuner_server_status" table
ALTER TABLE "tuner_server_status" ADD COLUMN "broadcast_clock_skew_ms" bigint NULL, ADD COLUMN "broadcast_clock_checked_at" timestamptz NULL;

-- +goose Down
-- reverse: modify "tuner_server_status" table
ALTER TABLE "tuner_server_status" DROP COLUMN "broadcast_clock_checked_at", DROP COLUMN "broadcast_clock_skew_ms";
