-- +goose Up
-- create "channel_types" table
CREATE TABLE "channel_types" ("type" text NOT NULL, "display_name" text NOT NULL, "sort_order" integer NOT NULL, PRIMARY KEY ("type"));
-- create "epg_changes" table
CREATE TABLE "epg_changes" ("version" bigint NOT NULL, "service_id" bigint NOT NULL, "start_at" timestamptz NOT NULL, "end_at" timestamptz NOT NULL, "created_at" timestamptz NOT NULL);
-- create index "epg_changes_created_at_idx" to table: "epg_changes"
CREATE INDEX "epg_changes_created_at_idx" ON "epg_changes" ("created_at");
-- create index "epg_changes_version_idx" to table: "epg_changes"
CREATE INDEX "epg_changes_version_idx" ON "epg_changes" ("version");
-- create "services" table
CREATE TABLE "services" ("id" bigint NOT NULL, "network_id" bigint NOT NULL, "sid" bigint NOT NULL, "name" text NOT NULL, "name_normalized" text NOT NULL, "service_type" bigint NOT NULL, "channel_type" text NOT NULL, "channel" text NOT NULL, "remote_control_key_id" bigint NULL, "logo_id" bigint NULL, "hidden" boolean NOT NULL DEFAULT false, "sort_order" integer NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "services_channel_type_fkey" FOREIGN KEY ("channel_type") REFERENCES "channel_types" ("type") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- create "programs" table
CREATE TABLE "programs" ("service_id" bigint NOT NULL, "event_id" bigint NOT NULL, "mirakurun_id" bigint NOT NULL, "start_at" timestamptz NOT NULL, "end_at" timestamptz NOT NULL, "end_undecided" boolean NOT NULL, "is_free" boolean NOT NULL, "name" text NOT NULL, "description" text NOT NULL, "name_normalized" text NOT NULL, "description_normalized" text NOT NULL, "extended" jsonb NOT NULL, "genres" jsonb NOT NULL, "related_items" jsonb NOT NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("service_id", "event_id"), CONSTRAINT "programs_service_id_fkey" FOREIGN KEY ("service_id") REFERENCES "services" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION);
-- create index "programs_service_id_start_at_idx" to table: "programs"
CREATE INDEX "programs_service_id_start_at_idx" ON "programs" ("service_id", "start_at");
-- create index "programs_start_at_idx" to table: "programs"
CREATE INDEX "programs_start_at_idx" ON "programs" ("start_at");

-- +goose Down
-- reverse: create index "programs_start_at_idx" to table: "programs"
DROP INDEX "programs_start_at_idx";
-- reverse: create index "programs_service_id_start_at_idx" to table: "programs"
DROP INDEX "programs_service_id_start_at_idx";
-- reverse: create "programs" table
DROP TABLE "programs";
-- reverse: create "services" table
DROP TABLE "services";
-- reverse: create index "epg_changes_version_idx" to table: "epg_changes"
DROP INDEX "epg_changes_version_idx";
-- reverse: create index "epg_changes_created_at_idx" to table: "epg_changes"
DROP INDEX "epg_changes_created_at_idx";
-- reverse: create "epg_changes" table
DROP TABLE "epg_changes";
-- reverse: create "channel_types" table
DROP TABLE "channel_types";
