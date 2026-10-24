-- +goose Up
-- modify "recordings" table
ALTER TABLE "recordings" ADD COLUMN "broadcast_started_at" timestamptz NULL, ADD COLUMN "broadcast_ended_at" timestamptz NULL, ADD COLUMN "not_broadcast" boolean NOT NULL DEFAULT false, ADD COLUMN "aired_event_id" bigint NULL;

-- +goose Down
-- reverse: modify "recordings" table
ALTER TABLE "recordings" DROP COLUMN "aired_event_id", DROP COLUMN "not_broadcast", DROP COLUMN "broadcast_ended_at", DROP COLUMN "broadcast_started_at";
