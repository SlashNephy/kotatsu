-- +goose Up
-- create index "recordings_recorded_from_id_idx" to table: "recordings"
CREATE INDEX "recordings_recorded_from_id_idx" ON "recordings" ("recorded_from" DESC, "id" DESC);
-- create "tags" table
CREATE TABLE "tags" ("id" uuid NOT NULL, "name" text NOT NULL, "created_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "tags_name_key" UNIQUE ("name"));
-- create "recording_tags" table
CREATE TABLE "recording_tags" ("recording_id" uuid NOT NULL, "tag_id" uuid NOT NULL, PRIMARY KEY ("recording_id", "tag_id"), CONSTRAINT "recording_tags_recording_id_fkey" FOREIGN KEY ("recording_id") REFERENCES "recordings" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "recording_tags_tag_id_fkey" FOREIGN KEY ("tag_id") REFERENCES "tags" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "recording_tags_tag_id_idx" to table: "recording_tags"
CREATE INDEX "recording_tags_tag_id_idx" ON "recording_tags" ("tag_id", "recording_id");
-- create "watch_progress" table
CREATE TABLE "watch_progress" ("user_id" uuid NOT NULL, "recording_id" uuid NOT NULL, "position_ms" bigint NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("user_id", "recording_id"), CONSTRAINT "watch_progress_recording_id_fkey" FOREIGN KEY ("recording_id") REFERENCES "recordings" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "watch_progress_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "watch_progress_position_ms_check" CHECK (position_ms >= 0));

-- +goose Down
-- reverse: create "watch_progress" table
DROP TABLE "watch_progress";
-- reverse: create index "recording_tags_tag_id_idx" to table: "recording_tags"
DROP INDEX "recording_tags_tag_id_idx";
-- reverse: create "recording_tags" table
DROP TABLE "recording_tags";
-- reverse: create "tags" table
DROP TABLE "tags";
-- reverse: create index "recordings_recorded_from_id_idx" to table: "recordings"
DROP INDEX "recordings_recorded_from_id_idx";
