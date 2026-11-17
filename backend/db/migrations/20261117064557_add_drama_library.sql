-- +goose Up
-- create "drama_watched_episodes" table
CREATE TABLE "drama_watched_episodes" ("user_id" uuid NOT NULL, "tmdb_series_id" integer NOT NULL, "season_number" integer NOT NULL, "episode_number" integer NOT NULL, "source" text NOT NULL, "watched_at" timestamptz NOT NULL, PRIMARY KEY ("user_id", "tmdb_series_id", "season_number", "episode_number"), CONSTRAINT "drama_watched_episodes_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "drama_watched_episodes_episode_number_check" CHECK (episode_number > 0), CONSTRAINT "drama_watched_episodes_season_number_check" CHECK (season_number >= 0), CONSTRAINT "drama_watched_episodes_source_check" CHECK (source = ANY (ARRAY['playback'::text, 'manual'::text])), CONSTRAINT "drama_watched_episodes_tmdb_series_id_check" CHECK (tmdb_series_id > 0));
-- create index "drama_watched_episodes_tmdb_series_id_idx" to table: "drama_watched_episodes"
CREATE INDEX "drama_watched_episodes_tmdb_series_id_idx" ON "drama_watched_episodes" ("tmdb_series_id", "season_number");
-- create "drama_watchlist_entries" table
CREATE TABLE "drama_watchlist_entries" ("user_id" uuid NOT NULL, "tmdb_series_id" integer NOT NULL, "status" text NOT NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("user_id", "tmdb_series_id"), CONSTRAINT "drama_watchlist_entries_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "drama_watchlist_entries_status_check" CHECK (status = ANY (ARRAY['watching'::text, 'wanna_watch'::text])), CONSTRAINT "drama_watchlist_entries_tmdb_series_id_check" CHECK (tmdb_series_id > 0));
-- create index "drama_watchlist_entries_tmdb_series_id_idx" to table: "drama_watchlist_entries"
CREATE INDEX "drama_watchlist_entries_tmdb_series_id_idx" ON "drama_watchlist_entries" ("tmdb_series_id");

-- +goose Down
-- reverse: create index "drama_watchlist_entries_tmdb_series_id_idx" to table: "drama_watchlist_entries"
DROP INDEX "drama_watchlist_entries_tmdb_series_id_idx";
-- reverse: create "drama_watchlist_entries" table
DROP TABLE "drama_watchlist_entries";
-- reverse: create index "drama_watched_episodes_tmdb_series_id_idx" to table: "drama_watched_episodes"
DROP INDEX "drama_watched_episodes_tmdb_series_id_idx";
-- reverse: create "drama_watched_episodes" table
DROP TABLE "drama_watched_episodes";
