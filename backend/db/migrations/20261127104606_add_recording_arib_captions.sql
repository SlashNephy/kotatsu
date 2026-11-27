-- +goose Up
-- create "recording_arib_captions" table
CREATE TABLE "recording_arib_captions" ("recording_id" uuid NOT NULL, "extractor_version" integer NOT NULL, "extracted_at" timestamptz NOT NULL, "cues" jsonb NOT NULL, "search_grams" text[] NOT NULL, PRIMARY KEY ("recording_id"), CONSTRAINT "recording_arib_captions_recording_id_fkey" FOREIGN KEY ("recording_id") REFERENCES "recordings" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "recording_arib_captions_search_grams_idx" to table: "recording_arib_captions"
CREATE INDEX "recording_arib_captions_search_grams_idx" ON "recording_arib_captions" USING gin ("search_grams");

-- +goose Down
-- reverse: create index "recording_arib_captions_search_grams_idx" to table: "recording_arib_captions"
DROP INDEX "recording_arib_captions_search_grams_idx";
-- reverse: create "recording_arib_captions" table
DROP TABLE "recording_arib_captions";
