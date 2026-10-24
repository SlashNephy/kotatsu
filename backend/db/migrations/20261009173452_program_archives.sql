-- +goose Up
-- create index "programs_end_at_idx" to table: "programs"
CREATE INDEX "programs_end_at_idx" ON "programs" ("end_at");
-- create "program_archives" table
CREATE TABLE "program_archives" ("service_id" bigint NOT NULL, "event_id" bigint NOT NULL, "mirakurun_id" bigint NOT NULL, "start_at" timestamptz NOT NULL, "end_at" timestamptz NOT NULL, "end_undecided" boolean NOT NULL, "is_free" boolean NOT NULL, "name" text NOT NULL, "description" text NOT NULL, "name_normalized" text NOT NULL, "description_normalized" text NOT NULL, "extended" jsonb NOT NULL, "genres" jsonb NOT NULL, "related_items" jsonb NOT NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, "archived_at" timestamptz NOT NULL, PRIMARY KEY ("service_id", "event_id", "start_at"), CONSTRAINT "program_archives_service_id_fkey" FOREIGN KEY ("service_id") REFERENCES "services" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- create index "program_archives_end_at_idx" to table: "program_archives"
CREATE INDEX "program_archives_end_at_idx" ON "program_archives" ("end_at");
-- create index "program_archives_service_id_start_at_idx" to table: "program_archives"
CREATE INDEX "program_archives_service_id_start_at_idx" ON "program_archives" ("service_id", "start_at");
-- create index "program_archives_start_at_idx" to table: "program_archives"
CREATE INDEX "program_archives_start_at_idx" ON "program_archives" ("start_at");

-- +goose Down
-- reverse: create index "program_archives_start_at_idx" to table: "program_archives"
DROP INDEX "program_archives_start_at_idx";
-- reverse: create index "program_archives_service_id_start_at_idx" to table: "program_archives"
DROP INDEX "program_archives_service_id_start_at_idx";
-- reverse: create index "program_archives_end_at_idx" to table: "program_archives"
DROP INDEX "program_archives_end_at_idx";
-- reverse: create "program_archives" table
DROP TABLE "program_archives";
-- reverse: create index "programs_end_at_idx" to table: "programs"
DROP INDEX "programs_end_at_idx";
