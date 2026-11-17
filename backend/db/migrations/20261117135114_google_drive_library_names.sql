-- +goose Up
-- modify "google_drive_folders" table
-- 選んだフォルダごとにライブラリを作る (#1902)。前に選んだフォルダには、google-drive-1、google-drive-2 と、選んだ順の名前を付ける。
-- 管理者は、連携の画面で名前を変えられる。
ALTER TABLE "google_drive_folders" ADD COLUMN "library_name" text NULL;
UPDATE "google_drive_folders" AS f SET "library_name" = 'google-drive-' || n.position
FROM (SELECT "folder_id", row_number() OVER (ORDER BY "added_at", "folder_id") AS position FROM "google_drive_folders") AS n
WHERE f."folder_id" = n."folder_id";
ALTER TABLE "google_drive_folders" ALTER COLUMN "library_name" SET NOT NULL, ADD CONSTRAINT "google_drive_folders_library_name_check" CHECK (library_name ~ '^[a-z0-9][a-z0-9_-]{0,62}$'::text), ADD CONSTRAINT "google_drive_folders_library_name_key" UNIQUE ("library_name");

-- +goose Down
-- reverse: modify "google_drive_folders" table
ALTER TABLE "google_drive_folders" DROP CONSTRAINT "google_drive_folders_library_name_key", DROP COLUMN "library_name", DROP CONSTRAINT "google_drive_folders_library_name_check";
