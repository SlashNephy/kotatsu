-- +goose Up
-- modify "system_settings" table
ALTER TABLE "system_settings" ADD CONSTRAINT "system_settings_timeshift_while_watching_retention_second_check" CHECK ((timeshift_while_watching_retention_seconds >= 600) AND (timeshift_while_watching_retention_seconds <= 86400)), ADD COLUMN "timeshift_while_watching" boolean NOT NULL DEFAULT false, ADD COLUMN "timeshift_while_watching_retention_seconds" integer NOT NULL DEFAULT 3600;
-- modify "timeshift_channels" table
ALTER TABLE "timeshift_channels" ADD COLUMN "while_watching" boolean NOT NULL DEFAULT false;
-- create "timeshift_watches" table
CREATE TABLE "timeshift_watches" ("service_id" bigint NOT NULL, "watching_until" timestamptz NOT NULL, PRIMARY KEY ("service_id"), CONSTRAINT "timeshift_watches_service_id_fkey" FOREIGN KEY ("service_id") REFERENCES "services" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);

-- +goose Down
-- reverse: create "timeshift_watches" table
DROP TABLE "timeshift_watches";
-- reverse: modify "timeshift_channels" table
ALTER TABLE "timeshift_channels" DROP COLUMN "while_watching";
-- reverse: modify "system_settings" table
ALTER TABLE "system_settings" DROP COLUMN "timeshift_while_watching_retention_seconds", DROP COLUMN "timeshift_while_watching", DROP CONSTRAINT "system_settings_timeshift_while_watching_retention_second_check";
