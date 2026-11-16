-- +goose Up
-- create "library_watch_progress" table
CREATE TABLE "library_watch_progress" ("user_id" uuid NOT NULL, "file_id" text NOT NULL, "position_ms" bigint NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("user_id", "file_id"), CONSTRAINT "library_watch_progress_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "library_watch_progress_file_id_check" CHECK (file_id ~ '^[0-9a-f]{32}$'::text), CONSTRAINT "library_watch_progress_position_ms_check" CHECK (position_ms >= 0));
-- create index "library_watch_progress_updated_at_idx" to table: "library_watch_progress"
CREATE INDEX "library_watch_progress_updated_at_idx" ON "library_watch_progress" ("updated_at");

-- +goose Down
-- reverse: create index "library_watch_progress_updated_at_idx" to table: "library_watch_progress"
DROP INDEX "library_watch_progress_updated_at_idx";
-- reverse: create "library_watch_progress" table
DROP TABLE "library_watch_progress";
