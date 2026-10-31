-- +goose Up
-- create "recording_live_stats" table
CREATE TABLE "recording_live_stats" ("recording_id" uuid NOT NULL, "bytes" bigint NOT NULL, "drops" bigint NOT NULL, "errors" bigint NOT NULL, "scrambled" bigint NOT NULL, "bitrate" bigint NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("recording_id"), CONSTRAINT "recording_live_stats_recording_id_fkey" FOREIGN KEY ("recording_id") REFERENCES "recordings" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);

-- +goose Down
-- reverse: create "recording_live_stats" table
DROP TABLE "recording_live_stats";
