-- +goose Up
-- create "program_rewrites" table
CREATE TABLE "program_rewrites" ("service_id" bigint NOT NULL, "event_id" bigint NOT NULL, "start_at" timestamptz NOT NULL, "field" text NOT NULL, "before" text NOT NULL, "after" text NOT NULL, "observed_at" timestamptz NOT NULL, CONSTRAINT "program_rewrites_field_check" CHECK (field = ANY (ARRAY['name'::text, 'description'::text])));
-- create index "program_rewrites_observed_at_idx" to table: "program_rewrites"
CREATE INDEX "program_rewrites_observed_at_idx" ON "program_rewrites" ("observed_at");

-- +goose Down
-- reverse: create index "program_rewrites_observed_at_idx" to table: "program_rewrites"
DROP INDEX "program_rewrites_observed_at_idx";
-- reverse: create "program_rewrites" table
DROP TABLE "program_rewrites";
