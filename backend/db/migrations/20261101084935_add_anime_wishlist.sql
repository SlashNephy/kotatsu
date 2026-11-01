-- +goose Up
-- modify "anime_record_connections" table
ALTER TABLE "anime_record_connections" ADD COLUMN "wishlist_checked_at" timestamptz NULL, ADD COLUMN "wishlist_imported_at" timestamptz NULL, ADD COLUMN "wishlist_error" text NULL, ADD COLUMN "wishlist_retry_at" timestamptz NULL;
-- create index "anime_works_anilist_id_idx" to table: "anime_works"
CREATE INDEX "anime_works_anilist_id_idx" ON "anime_works" ("anilist_id") WHERE (anilist_id IS NOT NULL);
-- create index "anime_works_annict_work_id_idx" to table: "anime_works"
CREATE INDEX "anime_works_annict_work_id_idx" ON "anime_works" ("annict_work_id") WHERE (annict_work_id IS NOT NULL);
-- create "anime_wishlist_entries" table
CREATE TABLE "anime_wishlist_entries" ("user_id" uuid NOT NULL, "destination" text NOT NULL, "work_id" integer NOT NULL, "status" text NOT NULL, "title" text NOT NULL, "fetched_at" timestamptz NOT NULL, PRIMARY KEY ("user_id", "destination", "work_id"), CONSTRAINT "anime_wishlist_entries_user_id_destination_fkey" FOREIGN KEY ("user_id", "destination") REFERENCES "anime_record_connections" ("user_id", "destination") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "anime_wishlist_entries_status_check" CHECK (status = ANY (ARRAY['wanna_watch'::text, 'watching'::text])));

-- +goose Down
-- reverse: create "anime_wishlist_entries" table
DROP TABLE "anime_wishlist_entries";
-- reverse: create index "anime_works_annict_work_id_idx" to table: "anime_works"
DROP INDEX "anime_works_annict_work_id_idx";
-- reverse: create index "anime_works_anilist_id_idx" to table: "anime_works"
DROP INDEX "anime_works_anilist_id_idx";
-- reverse: modify "anime_record_connections" table
ALTER TABLE "anime_record_connections" DROP COLUMN "wishlist_retry_at", DROP COLUMN "wishlist_error", DROP COLUMN "wishlist_imported_at", DROP COLUMN "wishlist_checked_at";
