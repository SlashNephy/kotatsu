-- +goose Up
-- create "system_settings" table
CREATE TABLE "system_settings" ("id" smallint NOT NULL, "recording_defaults" jsonb NOT NULL, "archive_retention_days" integer NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "system_settings_archive_retention_days_check" CHECK (archive_retention_days >= 0), CONSTRAINT "system_settings_id_check" CHECK (id = 1));

-- +goose Down
-- reverse: create "system_settings" table
DROP TABLE "system_settings";
