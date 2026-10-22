-- +goose Up
-- modify "audit_events" table
ALTER TABLE "audit_events" ADD COLUMN "actor_client" text NULL;

-- +goose Down
-- reverse: modify "audit_events" table
ALTER TABLE "audit_events" DROP COLUMN "actor_client";
