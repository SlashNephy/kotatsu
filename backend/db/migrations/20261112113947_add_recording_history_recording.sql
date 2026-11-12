-- +goose Up
-- modify "recording_history" table
ALTER TABLE "recording_history" ADD COLUMN "recording_id" uuid NULL, ADD CONSTRAINT "recording_history_recording_id_fkey" FOREIGN KEY ("recording_id") REFERENCES "recordings" ("id") ON UPDATE NO ACTION ON DELETE SET NULL;
-- create index "recording_history_recording_id_idx" to table: "recording_history"
CREATE INDEX "recording_history_recording_id_idx" ON "recording_history" ("recording_id");

-- +goose Down
-- reverse: create index "recording_history_recording_id_idx" to table: "recording_history"
DROP INDEX "recording_history_recording_id_idx";
-- reverse: modify "recording_history" table
ALTER TABLE "recording_history" DROP CONSTRAINT "recording_history_recording_id_fkey", DROP COLUMN "recording_id";
