-- +goose Up
-- create "backup_runs" table
CREATE TABLE "backup_runs" ("id" uuid NOT NULL, "trigger" text NOT NULL, "started_at" timestamptz NOT NULL, "finished_at" timestamptz NOT NULL, "file_name" text NULL, "size_bytes" bigint NULL, "error" text NULL, PRIMARY KEY ("id"), CONSTRAINT "backup_runs_check" CHECK ((error IS NULL) = (file_name IS NOT NULL)), CONSTRAINT "backup_runs_size_bytes_check" CHECK (size_bytes >= 0), CONSTRAINT "backup_runs_trigger_check" CHECK (trigger = ANY (ARRAY['schedule'::text, 'manual'::text])));
-- create index "backup_runs_started_at_idx" to table: "backup_runs"
CREATE INDEX "backup_runs_started_at_idx" ON "backup_runs" ("started_at" DESC);
-- create "backup_settings" table
CREATE TABLE "backup_settings" ("id" smallint NOT NULL, "recipients" text[] NOT NULL, "interval_hours" integer NOT NULL, "generations" integer NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "backup_settings_generations_check" CHECK ((generations >= 1) AND (generations <= 365)), CONSTRAINT "backup_settings_id_check" CHECK (id = 1), CONSTRAINT "backup_settings_interval_hours_check" CHECK ((interval_hours >= 1) AND (interval_hours <= 720)));

-- +goose Down
-- reverse: create "backup_settings" table
DROP TABLE "backup_settings";
-- reverse: create index "backup_runs_started_at_idx" to table: "backup_runs"
DROP INDEX "backup_runs_started_at_idx";
-- reverse: create "backup_runs" table
DROP TABLE "backup_runs";
