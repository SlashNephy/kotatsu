-- +goose Up
-- modify "notification_events" table
ALTER TABLE "notification_events" ADD COLUMN "audience" uuid[] NULL, ADD COLUMN "by_rule" boolean NOT NULL DEFAULT false;
-- modify "notification_preferences" table
ALTER TABLE "notification_preferences" ADD COLUMN "rule_reservations" boolean NOT NULL DEFAULT true;

-- +goose Down
-- reverse: modify "notification_preferences" table
ALTER TABLE "notification_preferences" DROP COLUMN "rule_reservations";
-- reverse: modify "notification_events" table
ALTER TABLE "notification_events" DROP COLUMN "by_rule", DROP COLUMN "audience";
