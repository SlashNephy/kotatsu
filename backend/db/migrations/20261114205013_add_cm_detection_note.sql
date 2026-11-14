-- +goose Up
-- modify "recording_cm_detections" table
ALTER TABLE "recording_cm_detections" ADD COLUMN "note" text NOT NULL DEFAULT '', ADD COLUMN "video_missing_ms" bigint NOT NULL DEFAULT 0;

-- +goose Down
-- reverse: modify "recording_cm_detections" table
ALTER TABLE "recording_cm_detections" DROP COLUMN "video_missing_ms", DROP COLUMN "note";
