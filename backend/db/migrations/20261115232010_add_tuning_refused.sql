-- +goose Up
-- modify "recording_failure_counts" table
ALTER TABLE "recording_failure_counts" DROP CONSTRAINT "recording_failure_counts_reason_check", ADD CONSTRAINT "recording_failure_counts_reason_check" CHECK (reason = ANY (ARRAY['tuner_unavailable'::text, 'tuner_server_unreachable'::text, 'stream_interrupted'::text, 'storage_unavailable'::text, 'storage_full'::text, 'program_not_found'::text, 'cancelled'::text, 'recorder_unavailable'::text, 'tuner_preempted'::text, 'tuning_refused'::text]));
-- modify "recordings" table
ALTER TABLE "recordings" DROP CONSTRAINT "recordings_failure_reason_check", ADD CONSTRAINT "recordings_failure_reason_check" CHECK (failure_reason = ANY (ARRAY['tuner_unavailable'::text, 'tuner_server_unreachable'::text, 'stream_interrupted'::text, 'storage_unavailable'::text, 'storage_full'::text, 'program_not_found'::text, 'cancelled'::text, 'recorder_unavailable'::text, 'tuner_preempted'::text, 'tuning_refused'::text]));
-- 失敗の理由ごとの累計に、0 件の行を足す。
INSERT INTO "recording_failure_counts" ("reason") VALUES ('tuning_refused');

-- +goose Down
DELETE FROM "recording_failure_counts" WHERE "reason" = 'tuning_refused';
-- 選局を断られた録画は、分ける前と同じく、チューナーを確保できなかったものに戻す。
UPDATE "recordings" SET "failure_reason" = 'tuner_unavailable' WHERE "failure_reason" = 'tuning_refused';
-- reverse: modify "recordings" table
ALTER TABLE "recordings" DROP CONSTRAINT "recordings_failure_reason_check", ADD CONSTRAINT "recordings_failure_reason_check" CHECK (failure_reason = ANY (ARRAY['tuner_unavailable'::text, 'tuner_server_unreachable'::text, 'stream_interrupted'::text, 'storage_unavailable'::text, 'storage_full'::text, 'program_not_found'::text, 'cancelled'::text, 'recorder_unavailable'::text, 'tuner_preempted'::text]));
-- reverse: modify "recording_failure_counts" table
ALTER TABLE "recording_failure_counts" DROP CONSTRAINT "recording_failure_counts_reason_check", ADD CONSTRAINT "recording_failure_counts_reason_check" CHECK (reason = ANY (ARRAY['tuner_unavailable'::text, 'tuner_server_unreachable'::text, 'stream_interrupted'::text, 'storage_unavailable'::text, 'storage_full'::text, 'program_not_found'::text, 'cancelled'::text, 'recorder_unavailable'::text, 'tuner_preempted'::text]));
