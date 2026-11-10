-- +goose Up
-- create "recording_nicolog_files" table
CREATE TABLE "recording_nicolog_files" ("recording_id" uuid NOT NULL, "path" text NULL, "checked_at" timestamptz NOT NULL, PRIMARY KEY ("recording_id"), CONSTRAINT "recording_nicolog_files_recording_id_fkey" FOREIGN KEY ("recording_id") REFERENCES "recordings" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);

-- +goose Down
-- reverse: create "recording_nicolog_files" table
DROP TABLE "recording_nicolog_files";
