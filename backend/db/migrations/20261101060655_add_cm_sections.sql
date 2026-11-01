-- +goose Up
-- create "recording_cm_detections" table
CREATE TABLE "recording_cm_detections" ("recording_id" uuid NOT NULL, "detector" text NOT NULL, "detector_version" integer NOT NULL, "detected_at" timestamptz NOT NULL, PRIMARY KEY ("recording_id", "detector"), CONSTRAINT "recording_cm_detections_recording_id_fkey" FOREIGN KEY ("recording_id") REFERENCES "recordings" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create "recording_cm_sections" table
CREATE TABLE "recording_cm_sections" ("recording_id" uuid NOT NULL, "detector" text NOT NULL, "start_offset_ms" bigint NOT NULL, "end_offset_ms" bigint NOT NULL, PRIMARY KEY ("recording_id", "detector", "start_offset_ms"), CONSTRAINT "recording_cm_sections_recording_id_detector_fkey" FOREIGN KEY ("recording_id", "detector") REFERENCES "recording_cm_detections" ("recording_id", "detector") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "recording_cm_sections_check" CHECK (end_offset_ms > start_offset_ms), CONSTRAINT "recording_cm_sections_start_offset_ms_check" CHECK (start_offset_ms >= 0));

-- +goose Down
-- reverse: create "recording_cm_sections" table
DROP TABLE "recording_cm_sections";
-- reverse: create "recording_cm_detections" table
DROP TABLE "recording_cm_detections";
