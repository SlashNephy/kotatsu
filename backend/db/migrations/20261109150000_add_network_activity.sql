-- +goose Up
-- modify "system_settings" table
ALTER TABLE "system_settings" ADD CONSTRAINT "system_settings_network_activity_retention_days_check" CHECK (network_activity_retention_days >= 1), ADD COLUMN "network_activity_enabled" boolean NOT NULL DEFAULT false, ADD COLUMN "network_activity_enabled_at" timestamptz NULL, ADD COLUMN "network_activity_retention_days" integer NOT NULL DEFAULT 30;
-- create "network_activity" table
CREATE TABLE "network_activity" ("hour" timestamptz NOT NULL, "purpose" text NOT NULL, "host" text NOT NULL, "initiator" text NOT NULL, "success" boolean NOT NULL, "count" bigint NOT NULL, "sent_bytes" bigint NOT NULL, "received_bytes" bigint NOT NULL, PRIMARY KEY ("hour", "purpose", "host", "initiator", "success"));

-- +goose Down
-- reverse: create "network_activity" table
DROP TABLE "network_activity";
-- reverse: modify "system_settings" table
ALTER TABLE "system_settings" DROP COLUMN "network_activity_retention_days", DROP COLUMN "network_activity_enabled_at", DROP COLUMN "network_activity_enabled", DROP CONSTRAINT "system_settings_network_activity_retention_days_check";
