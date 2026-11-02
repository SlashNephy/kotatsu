-- +goose Up
-- modify "program_anime_episodes" table
ALTER TABLE "program_anime_episodes" ADD COLUMN "is_rerun" boolean NOT NULL DEFAULT false;
-- modify "recording_anime_episodes" table
ALTER TABLE "recording_anime_episodes" ADD COLUMN "is_rerun" boolean NOT NULL DEFAULT false;

-- +goose Down
-- reverse: modify "recording_anime_episodes" table
ALTER TABLE "recording_anime_episodes" DROP COLUMN "is_rerun";
-- reverse: modify "program_anime_episodes" table
ALTER TABLE "program_anime_episodes" DROP COLUMN "is_rerun";
