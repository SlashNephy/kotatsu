-- +goose Up
-- modify "programs" table
ALTER TABLE "programs" ADD COLUMN "series" text NULL, ADD COLUMN "episode_number" integer NULL;
-- create index "programs_series_start_at_idx" to table: "programs"
CREATE INDEX "programs_series_start_at_idx" ON "programs" ("series", "start_at") WHERE (episode_number IS NOT NULL);

-- +goose Down
-- reverse: create index "programs_series_start_at_idx" to table: "programs"
DROP INDEX "programs_series_start_at_idx";
-- reverse: modify "programs" table
ALTER TABLE "programs" DROP COLUMN "episode_number", DROP COLUMN "series";
