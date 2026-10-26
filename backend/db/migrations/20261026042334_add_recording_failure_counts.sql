-- +goose Up
-- create "recording_failure_counts" table
CREATE TABLE "recording_failure_counts" ("reason" text NOT NULL, "count" bigint NOT NULL DEFAULT 0, "last_failed_at" timestamptz NULL, PRIMARY KEY ("reason"), CONSTRAINT "recording_failure_counts_reason_check" CHECK (reason = ANY (ARRAY['tuner_unavailable'::text, 'tuner_server_unreachable'::text, 'stream_interrupted'::text, 'storage_unavailable'::text, 'storage_full'::text, 'program_not_found'::text, 'cancelled'::text, 'recorder_unavailable'::text])));
-- 理由ごとに 0 件の行を作っておき、一度も失敗していない理由も /metrics に 0 として出す (#960)。今ある失敗の録画は足さない。
INSERT INTO "recording_failure_counts" ("reason") VALUES
  ('tuner_unavailable'), ('tuner_server_unreachable'), ('stream_interrupted'), ('storage_unavailable'),
  ('storage_full'), ('program_not_found'), ('cancelled'), ('recorder_unavailable');

-- +goose Down
-- reverse: create "recording_failure_counts" table
DROP TABLE "recording_failure_counts";
