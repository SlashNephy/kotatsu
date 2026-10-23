-- +goose Up
-- create "recording_history" table
CREATE TABLE "recording_history" ("id" uuid NOT NULL, "service_id" bigint NOT NULL, "title_key" text NOT NULL, "name" text NOT NULL, "start_at" timestamptz NOT NULL, "end_at" timestamptz NOT NULL, "created_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "recording_history_service_id_fkey" FOREIGN KEY ("service_id") REFERENCES "services" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- create index "recording_history_title_key_service_id_end_at_idx" to table: "recording_history"
CREATE INDEX "recording_history_title_key_service_id_end_at_idx" ON "recording_history" ("title_key", "service_id", "end_at");
-- create "rules" table
CREATE TABLE "rules" ("id" uuid NOT NULL, "name" text NOT NULL, "memo" text NOT NULL, "enabled" boolean NOT NULL, "conditions" jsonb NOT NULL, "settings" jsonb NOT NULL, "valid_from" timestamptz NULL, "valid_until" timestamptz NULL, "avoid_duplicates" boolean NOT NULL, "duplicate_window_days" integer NULL, "follow_event_relay" boolean NOT NULL, "created_at" timestamptz NOT NULL, "created_by" uuid NULL, "updated_at" timestamptz NOT NULL, "updated_by" uuid NULL, PRIMARY KEY ("id"), CONSTRAINT "rules_created_by_fkey" FOREIGN KEY ("created_by") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE SET NULL, CONSTRAINT "rules_updated_by_fkey" FOREIGN KEY ("updated_by") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE SET NULL, CONSTRAINT "rules_duplicate_window_check" CHECK ((duplicate_window_days IS NULL) OR (duplicate_window_days > 0)), CONSTRAINT "rules_valid_period_check" CHECK ((valid_from IS NULL) OR (valid_until IS NULL) OR (valid_from < valid_until)));
-- create "reservation_rules" table
CREATE TABLE "reservation_rules" ("reservation_id" uuid NOT NULL, "rule_id" uuid NOT NULL, PRIMARY KEY ("reservation_id", "rule_id"), CONSTRAINT "reservation_rules_reservation_id_fkey" FOREIGN KEY ("reservation_id") REFERENCES "reservations" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "reservation_rules_rule_id_fkey" FOREIGN KEY ("rule_id") REFERENCES "rules" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "reservation_rules_rule_id_idx" to table: "reservation_rules"
CREATE INDEX "reservation_rules_rule_id_idx" ON "reservation_rules" ("rule_id");

-- +goose Down
-- reverse: create index "reservation_rules_rule_id_idx" to table: "reservation_rules"
DROP INDEX "reservation_rules_rule_id_idx";
-- reverse: create "reservation_rules" table
DROP TABLE "reservation_rules";
-- reverse: create "rules" table
DROP TABLE "rules";
-- reverse: create index "recording_history_title_key_service_id_end_at_idx" to table: "recording_history"
DROP INDEX "recording_history_title_key_service_id_end_at_idx";
-- reverse: create "recording_history" table
DROP TABLE "recording_history";
