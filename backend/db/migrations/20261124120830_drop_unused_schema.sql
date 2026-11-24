-- +goose Up
-- modify "system_settings" table
ALTER TABLE "system_settings" DROP CONSTRAINT "system_settings_rule_unmatched_grace_minutes_check", DROP COLUMN "rule_unmatched_grace_minutes";
-- drop "program_rewrites" table
DROP TABLE "program_rewrites";

-- +goose Down
-- reverse: drop "program_rewrites" table
CREATE TABLE "program_rewrites" ("service_id" bigint NOT NULL, "event_id" bigint NOT NULL, "start_at" timestamptz NOT NULL, "field" text NOT NULL, "before" text NOT NULL, "after" text NOT NULL, "observed_at" timestamptz NOT NULL, CONSTRAINT "program_rewrites_field_check" CHECK (field = ANY (ARRAY['name'::text, 'description'::text])));
CREATE INDEX "program_rewrites_observed_at_idx" ON "program_rewrites" ("observed_at");
-- reverse: modify "system_settings" table
ALTER TABLE "system_settings" ADD COLUMN "rule_unmatched_grace_minutes" integer NOT NULL DEFAULT 180, ADD CONSTRAINT "system_settings_rule_unmatched_grace_minutes_check" CHECK ((rule_unmatched_grace_minutes >= 0) AND (rule_unmatched_grace_minutes <= 1440));
