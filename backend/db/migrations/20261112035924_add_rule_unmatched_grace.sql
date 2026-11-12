-- +goose Up
-- modify "system_settings" table
ALTER TABLE "system_settings" ADD CONSTRAINT "system_settings_rule_unmatched_grace_minutes_check" CHECK ((rule_unmatched_grace_minutes >= 0) AND (rule_unmatched_grace_minutes <= 1440)), ADD COLUMN "rule_unmatched_grace_minutes" integer NOT NULL DEFAULT 180;

-- +goose Down
-- reverse: modify "system_settings" table
ALTER TABLE "system_settings" DROP COLUMN "rule_unmatched_grace_minutes", DROP CONSTRAINT "system_settings_rule_unmatched_grace_minutes_check";
