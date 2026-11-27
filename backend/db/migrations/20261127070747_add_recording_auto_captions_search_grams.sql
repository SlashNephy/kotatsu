-- +goose Up
-- modify "recording_auto_captions" table
ALTER TABLE "recording_auto_captions" ADD COLUMN "search_grams" text[] NULL, ADD COLUMN "search_grams_generated_at" timestamptz NULL;
-- create index "recording_auto_captions_search_grams_idx" to table: "recording_auto_captions"
CREATE INDEX "recording_auto_captions_search_grams_idx" ON "recording_auto_captions" USING gin ("search_grams");
-- create index "recording_auto_captions_stale_search_grams_idx" to table: "recording_auto_captions"
CREATE INDEX "recording_auto_captions_stale_search_grams_idx" ON "recording_auto_captions" ("recording_id") WHERE (search_grams_generated_at IS DISTINCT FROM generated_at);

-- +goose Down
-- reverse: create index "recording_auto_captions_stale_search_grams_idx" to table: "recording_auto_captions"
DROP INDEX "recording_auto_captions_stale_search_grams_idx";
-- reverse: create index "recording_auto_captions_search_grams_idx" to table: "recording_auto_captions"
DROP INDEX "recording_auto_captions_search_grams_idx";
-- reverse: modify "recording_auto_captions" table
ALTER TABLE "recording_auto_captions" DROP COLUMN "search_grams_generated_at", DROP COLUMN "search_grams";
