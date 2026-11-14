-- +goose Up
-- create "recording_cm_materials" table
CREATE TABLE "recording_cm_materials" ("recording_id" uuid NOT NULL, "detector" text NOT NULL, "format_version" integer NOT NULL, "data" bytea NOT NULL, "created_at" timestamptz NOT NULL, PRIMARY KEY ("recording_id", "detector"), CONSTRAINT "recording_cm_materials_recording_id_fkey" FOREIGN KEY ("recording_id") REFERENCES "recordings" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);

-- +goose Down
-- reverse: create "recording_cm_materials" table
DROP TABLE "recording_cm_materials";
