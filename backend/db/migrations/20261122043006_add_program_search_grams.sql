-- +goose Up
-- modify "program_archives" table
ALTER TABLE "program_archives" ADD COLUMN "search_grams" text[] NULL;
-- create index "program_archives_search_grams_idx" to table: "program_archives"
CREATE INDEX "program_archives_search_grams_idx" ON "program_archives" USING gin ("search_grams");
-- create index "program_archives_search_grams_null_idx" to table: "program_archives"
CREATE INDEX "program_archives_search_grams_null_idx" ON "program_archives" ("service_id", "event_id", "start_at") WHERE (search_grams IS NULL);
-- modify "programs" table
ALTER TABLE "programs" ADD COLUMN "search_grams" text[] NULL;
-- create index "programs_search_grams_idx" to table: "programs"
CREATE INDEX "programs_search_grams_idx" ON "programs" USING gin ("search_grams");
-- create index "programs_search_grams_null_idx" to table: "programs"
CREATE INDEX "programs_search_grams_null_idx" ON "programs" ("service_id", "event_id") WHERE (search_grams IS NULL);

-- +goose Down
-- reverse: create index "programs_search_grams_null_idx" to table: "programs"
DROP INDEX "programs_search_grams_null_idx";
-- reverse: create index "programs_search_grams_idx" to table: "programs"
DROP INDEX "programs_search_grams_idx";
-- reverse: modify "programs" table
ALTER TABLE "programs" DROP COLUMN "search_grams";
-- reverse: create index "program_archives_search_grams_null_idx" to table: "program_archives"
DROP INDEX "program_archives_search_grams_null_idx";
-- reverse: create index "program_archives_search_grams_idx" to table: "program_archives"
DROP INDEX "program_archives_search_grams_idx";
-- reverse: modify "program_archives" table
ALTER TABLE "program_archives" DROP COLUMN "search_grams";
