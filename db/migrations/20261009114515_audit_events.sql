-- +goose Up
-- create "audit_events" table
CREATE TABLE "audit_events" ("id" uuid NOT NULL, "occurred_at" timestamptz NOT NULL, "actor_type" text NOT NULL, "actor_user_id" uuid NULL, "actor_role" text NULL, "action" text NOT NULL, "target_type" text NOT NULL, "target_id" text NULL, "summary" jsonb NOT NULL, "source_ip" inet NULL, "user_agent" text NULL, PRIMARY KEY ("id"), CONSTRAINT "audit_events_actor_check" CHECK (((actor_type = 'user'::text) AND (actor_user_id IS NOT NULL) AND (actor_role IS NULL)) OR ((actor_type = 'system'::text) AND (actor_user_id IS NULL) AND (actor_role IS NOT NULL))));
-- create index "audit_events_occurred_at_idx" to table: "audit_events"
CREATE INDEX "audit_events_occurred_at_idx" ON "audit_events" ("occurred_at");

-- +goose Down
-- reverse: create index "audit_events_occurred_at_idx" to table: "audit_events"
DROP INDEX "audit_events_occurred_at_idx";
-- reverse: create "audit_events" table
DROP TABLE "audit_events";
