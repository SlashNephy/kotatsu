-- +goose Up
-- modify "reservations" table
ALTER TABLE "reservations" ADD CONSTRAINT "reservations_disabled_reason_check" CHECK (disabled_reason = 'recorded_episode'::text), ADD COLUMN "disabled_reason" text NULL, ADD COLUMN "disabled_service_id" bigint NULL, ADD COLUMN "episode_recheck_waived" boolean NOT NULL DEFAULT false;

-- +goose Down
-- reverse: modify "reservations" table
ALTER TABLE "reservations" DROP COLUMN "episode_recheck_waived", DROP COLUMN "disabled_service_id", DROP COLUMN "disabled_reason", DROP CONSTRAINT "reservations_disabled_reason_check";
