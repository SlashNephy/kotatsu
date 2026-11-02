-- +goose Up
-- create index "program_anime_episodes_syobocal_tid_idx" to table: "program_anime_episodes"
CREATE INDEX "program_anime_episodes_syobocal_tid_idx" ON "program_anime_episodes" ("syobocal_tid") WHERE (syobocal_tid IS NOT NULL);
-- create index "recording_anime_episodes_syobocal_tid_idx" to table: "recording_anime_episodes"
CREATE INDEX "recording_anime_episodes_syobocal_tid_idx" ON "recording_anime_episodes" ("syobocal_tid") WHERE (syobocal_tid IS NOT NULL);

-- +goose Down
-- reverse: create index "recording_anime_episodes_syobocal_tid_idx" to table: "recording_anime_episodes"
DROP INDEX "recording_anime_episodes_syobocal_tid_idx";
-- reverse: create index "program_anime_episodes_syobocal_tid_idx" to table: "program_anime_episodes"
DROP INDEX "program_anime_episodes_syobocal_tid_idx";
