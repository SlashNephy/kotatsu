-- +goose Up
-- modify "recordings" table
ALTER TABLE "recordings" DROP CONSTRAINT "recordings_failure_reason_check", ADD CONSTRAINT "recordings_failure_reason_check" CHECK (failure_reason = ANY (ARRAY['tuner_unavailable'::text, 'tuner_server_unreachable'::text, 'stream_interrupted'::text, 'storage_unavailable'::text, 'storage_full'::text, 'program_not_found'::text, 'cancelled'::text, 'recorder_unavailable'::text]));

-- +goose Down
-- reverse: modify "recordings" table
ALTER TABLE "recordings" DROP CONSTRAINT "recordings_failure_reason_check", ADD CONSTRAINT "recordings_failure_reason_check" CHECK (failure_reason = ANY (ARRAY['tuner_unavailable'::text, 'tuner_server_unreachable'::text, 'stream_interrupted'::text, 'storage_unavailable'::text, 'storage_full'::text, 'program_not_found'::text, 'cancelled'::text]));
