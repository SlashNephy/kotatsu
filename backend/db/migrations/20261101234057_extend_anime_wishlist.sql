-- +goose Up
-- modify "anime_wishlist_entries" table
ALTER TABLE "anime_wishlist_entries" ADD CONSTRAINT "anime_wishlist_entries_progress_check" CHECK (progress >= 0), ADD CONSTRAINT "anime_wishlist_entries_season_name_check" CHECK (season_name = ANY (ARRAY['winter'::text, 'spring'::text, 'summer'::text, 'autumn'::text])), ADD COLUMN "next_episode_id" text NULL, ADD COLUMN "next_episode_number" integer NULL, ADD COLUMN "next_episode_number_text" text NULL, ADD COLUMN "next_episode_title" text NULL, ADD COLUMN "progress" integer NULL, ADD COLUMN "episodes_count" integer NULL, ADD COLUMN "season_year" integer NULL, ADD COLUMN "season_name" text NULL, ADD COLUMN "image_url" text NULL;

-- +goose Down
-- reverse: modify "anime_wishlist_entries" table
ALTER TABLE "anime_wishlist_entries" DROP COLUMN "image_url", DROP COLUMN "season_name", DROP COLUMN "season_year", DROP COLUMN "episodes_count", DROP COLUMN "progress", DROP COLUMN "next_episode_title", DROP COLUMN "next_episode_number_text", DROP COLUMN "next_episode_number", DROP COLUMN "next_episode_id", DROP CONSTRAINT "anime_wishlist_entries_season_name_check", DROP CONSTRAINT "anime_wishlist_entries_progress_check";
