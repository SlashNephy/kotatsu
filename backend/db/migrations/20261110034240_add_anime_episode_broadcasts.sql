-- +goose Up
-- modify "anime_channel_definitions" table
ALTER TABLE "anime_channel_definitions" ADD COLUMN "jikkyo_channel_id" integer NULL;
-- create "anime_episode_broadcasts" table
CREATE TABLE "anime_episode_broadcasts" ("pid" integer NOT NULL, "tid" integer NOT NULL, "channel_id" integer NOT NULL, "start_at" timestamptz NOT NULL, "end_at" timestamptz NOT NULL, "offset_seconds" integer NOT NULL, "episode_count" double precision NOT NULL, PRIMARY KEY ("pid"));
-- create index "anime_episode_broadcasts_tid_episode_count_idx" to table: "anime_episode_broadcasts"
CREATE INDEX "anime_episode_broadcasts_tid_episode_count_idx" ON "anime_episode_broadcasts" ("tid", "episode_count");
-- create "anime_title_broadcast_lookups" table
CREATE TABLE "anime_title_broadcast_lookups" ("tid" integer NOT NULL, "fetched_at" timestamptz NOT NULL, PRIMARY KEY ("tid"));

-- +goose Down
-- reverse: create "anime_title_broadcast_lookups" table
DROP TABLE "anime_title_broadcast_lookups";
-- reverse: create index "anime_episode_broadcasts_tid_episode_count_idx" to table: "anime_episode_broadcasts"
DROP INDEX "anime_episode_broadcasts_tid_episode_count_idx";
-- reverse: create "anime_episode_broadcasts" table
DROP TABLE "anime_episode_broadcasts";
-- reverse: modify "anime_channel_definitions" table
ALTER TABLE "anime_channel_definitions" DROP COLUMN "jikkyo_channel_id";
