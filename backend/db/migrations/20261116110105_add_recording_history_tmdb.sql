-- +goose Up
-- modify "recording_history" table
ALTER TABLE "recording_history" ADD COLUMN "tmdb_series_id" integer NULL, ADD COLUMN "tmdb_season_number" integer NULL, ADD COLUMN "tmdb_episode_number" integer NULL;
-- create index "recording_history_tmdb_series_id_idx" to table: "recording_history"
CREATE INDEX "recording_history_tmdb_series_id_idx" ON "recording_history" ("tmdb_series_id") WHERE (tmdb_series_id IS NOT NULL);

-- +goose Down
-- reverse: create index "recording_history_tmdb_series_id_idx" to table: "recording_history"
DROP INDEX "recording_history_tmdb_series_id_idx";
-- reverse: modify "recording_history" table
ALTER TABLE "recording_history" DROP COLUMN "tmdb_episode_number", DROP COLUMN "tmdb_season_number", DROP COLUMN "tmdb_series_id";
