-- +goose Up
-- create "recordings" table
CREATE TABLE "recordings" ("id" uuid NOT NULL, "reservation_id" uuid NULL, "service_id" bigint NOT NULL, "event_id" bigint NULL, "name" text NOT NULL, "description" text NOT NULL, "start_at" timestamptz NOT NULL, "end_at" timestamptz NOT NULL, "state" text NOT NULL, "failure_reason" text NULL, "protected" boolean NOT NULL DEFAULT false, "drops" bigint NOT NULL DEFAULT 0, "errors" bigint NOT NULL DEFAULT 0, "scrambled" bigint NOT NULL DEFAULT 0, "recorded_from" timestamptz NOT NULL, "recorded_until" timestamptz NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "recordings_reservation_id_key" UNIQUE ("reservation_id"), CONSTRAINT "recordings_reservation_id_fkey" FOREIGN KEY ("reservation_id") REFERENCES "reservations" ("id") ON UPDATE NO ACTION ON DELETE SET NULL, CONSTRAINT "recordings_service_id_fkey" FOREIGN KEY ("service_id") REFERENCES "services" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION, CONSTRAINT "recordings_failure_reason_check" CHECK (failure_reason = ANY (ARRAY['tuner_unavailable'::text, 'tuner_server_unreachable'::text, 'stream_interrupted'::text, 'storage_unavailable'::text, 'storage_full'::text, 'program_not_found'::text, 'cancelled'::text])), CONSTRAINT "recordings_state_check" CHECK (state = ANY (ARRAY['recording'::text, 'completed'::text, 'completed_with_errors'::text, 'partial'::text, 'failed'::text])));
-- create "recording_files" table
CREATE TABLE "recording_files" ("id" uuid NOT NULL, "recording_id" uuid NOT NULL, "kind" text NOT NULL, "pool_id" text NOT NULL, "path" text NOT NULL, "size" bigint NOT NULL, "started_at" timestamptz NOT NULL, "ended_at" timestamptz NULL, "created_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "recording_files_pool_id_path_key" UNIQUE ("pool_id", "path"), CONSTRAINT "recording_files_recording_id_fkey" FOREIGN KEY ("recording_id") REFERENCES "recordings" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "recording_files_kind_check" CHECK (kind = ANY (ARRAY['segment'::text, 'index'::text])));
-- create index "recording_files_recording_id_idx" to table: "recording_files"
CREATE INDEX "recording_files_recording_id_idx" ON "recording_files" ("recording_id", "started_at");

-- +goose Down
-- reverse: create index "recording_files_recording_id_idx" to table: "recording_files"
DROP INDEX "recording_files_recording_id_idx";
-- reverse: create "recording_files" table
DROP TABLE "recording_files";
-- reverse: create "recordings" table
DROP TABLE "recordings";
