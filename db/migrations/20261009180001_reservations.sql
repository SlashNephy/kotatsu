-- +goose Up
-- create "reservations" table
CREATE TABLE "reservations" ("id" uuid NOT NULL, "kind" text NOT NULL, "service_id" bigint NOT NULL, "event_id" bigint NULL, "start_at" timestamptz NOT NULL, "end_at" timestamptz NOT NULL, "name" text NOT NULL, "description" text NOT NULL, "extended" jsonb NOT NULL, "genres" jsonb NOT NULL, "enabled" boolean NOT NULL DEFAULT true, "follow_event_relay" boolean NOT NULL DEFAULT true, "settings" jsonb NOT NULL, "created_at" timestamptz NOT NULL, "created_by" uuid NULL, "updated_at" timestamptz NOT NULL, "updated_by" uuid NULL, PRIMARY KEY ("id"), CONSTRAINT "reservations_created_by_fkey" FOREIGN KEY ("created_by") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE SET NULL, CONSTRAINT "reservations_service_id_fkey" FOREIGN KEY ("service_id") REFERENCES "services" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION, CONSTRAINT "reservations_updated_by_fkey" FOREIGN KEY ("updated_by") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE SET NULL, CONSTRAINT "reservations_event_check" CHECK ((kind = 'time'::text) = (event_id IS NULL)), CONSTRAINT "reservations_kind_check" CHECK (kind = ANY (ARRAY['program'::text, 'time'::text, 'rule'::text])), CONSTRAINT "reservations_time_check" CHECK (start_at < end_at));
-- create index "reservations_program_idx" to table: "reservations"
CREATE UNIQUE INDEX "reservations_program_idx" ON "reservations" ("service_id", "event_id") WHERE (event_id IS NOT NULL);
-- create index "reservations_start_at_idx" to table: "reservations"
CREATE INDEX "reservations_start_at_idx" ON "reservations" ("start_at");

-- +goose Down
-- reverse: create index "reservations_start_at_idx" to table: "reservations"
DROP INDEX "reservations_start_at_idx";
-- reverse: create index "reservations_program_idx" to table: "reservations"
DROP INDEX "reservations_program_idx";
-- reverse: create "reservations" table
DROP TABLE "reservations";
