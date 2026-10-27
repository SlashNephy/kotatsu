-- +goose Up
-- create "timeshift_assignments" table
CREATE TABLE "timeshift_assignments" ("id" uuid NOT NULL, "service_id" bigint NOT NULL, "priority" integer NOT NULL, "state" text NOT NULL, "generation" bigint NOT NULL, "holder" uuid NULL, "lease_expires_at" timestamptz NULL, "stream_generation" bigint NOT NULL DEFAULT 0, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "timeshift_assignments_service_id_key" UNIQUE ("service_id"), CONSTRAINT "timeshift_assignments_service_id_fkey" FOREIGN KEY ("service_id") REFERENCES "services" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "timeshift_assignments_state_check" CHECK (state = ANY (ARRAY['waiting'::text, 'recording'::text, 'handover'::text, 'finished'::text])));
-- create "timeshift_channels" table
CREATE TABLE "timeshift_channels" ("service_id" bigint NOT NULL, "retention_seconds" integer NOT NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "updated_by" uuid NULL, PRIMARY KEY ("service_id"), CONSTRAINT "timeshift_channels_service_id_fkey" FOREIGN KEY ("service_id") REFERENCES "services" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "timeshift_channels_updated_by_fkey" FOREIGN KEY ("updated_by") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE SET NULL, CONSTRAINT "timeshift_channels_retention_seconds_check" CHECK ((retention_seconds >= 600) AND (retention_seconds <= 86400)));
-- create "timeshift_chunks" table
CREATE TABLE "timeshift_chunks" ("id" uuid NOT NULL, "service_id" bigint NOT NULL, "path" text NOT NULL, "size" bigint NOT NULL, "started_at" timestamptz NOT NULL, "ended_at" timestamptz NULL, "created_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "timeshift_chunks_path_key" UNIQUE ("path"), CONSTRAINT "timeshift_chunks_service_id_fkey" FOREIGN KEY ("service_id") REFERENCES "services" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "timeshift_chunks_service_started_at_idx" to table: "timeshift_chunks"
CREATE INDEX "timeshift_chunks_service_started_at_idx" ON "timeshift_chunks" ("service_id", "started_at");
-- create "timeshift_programs" table
CREATE TABLE "timeshift_programs" ("service_id" bigint NOT NULL, "started_at" timestamptz NOT NULL, "event_id" bigint NOT NULL, "name" text NOT NULL, "ended_at" timestamptz NULL, PRIMARY KEY ("service_id", "started_at"), CONSTRAINT "timeshift_programs_service_id_fkey" FOREIGN KEY ("service_id") REFERENCES "services" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);

-- +goose Down
-- reverse: create "timeshift_programs" table
DROP TABLE "timeshift_programs";
-- reverse: create index "timeshift_chunks_service_started_at_idx" to table: "timeshift_chunks"
DROP INDEX "timeshift_chunks_service_started_at_idx";
-- reverse: create "timeshift_chunks" table
DROP TABLE "timeshift_chunks";
-- reverse: create "timeshift_channels" table
DROP TABLE "timeshift_channels";
-- reverse: create "timeshift_assignments" table
DROP TABLE "timeshift_assignments";
