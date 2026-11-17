-- +goose Up
-- create "google_drive_folders" table
CREATE TABLE "google_drive_folders" ("folder_id" text NOT NULL, "drive_id" text NOT NULL, "name" text NOT NULL, "added_at" timestamptz NOT NULL, PRIMARY KEY ("folder_id"), CONSTRAINT "google_drive_folders_folder_id_check" CHECK (folder_id <> ''::text));

-- +goose Down
-- reverse: create "google_drive_folders" table
DROP TABLE "google_drive_folders";
