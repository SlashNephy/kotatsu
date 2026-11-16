-- +goose Up
-- create "recording_auto_captions" table
CREATE TABLE "recording_auto_captions" ("recording_id" uuid NOT NULL, "model" text NOT NULL, "generator_version" integer NOT NULL, "generated_at" timestamptz NOT NULL, "cues" jsonb NOT NULL, PRIMARY KEY ("recording_id"), CONSTRAINT "recording_auto_captions_recording_id_fkey" FOREIGN KEY ("recording_id") REFERENCES "recordings" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);

-- +goose Down
-- reverse: create "recording_auto_captions" table
DROP TABLE "recording_auto_captions";
