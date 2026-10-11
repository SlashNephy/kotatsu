-- name: CountRecordingsByState :many
-- /metrics に出す、状態ごとの録画の数 (設計書 11.3)。
SELECT state, count(*) AS count FROM recordings GROUP BY state ORDER BY state;

-- name: CountRecordingFailures :many
-- /metrics に出す、失敗の理由ごとの録画の数。
SELECT failure_reason::text AS reason, count(*) AS count FROM recordings WHERE failure_reason IS NOT NULL GROUP BY failure_reason ORDER BY failure_reason;

-- name: SumActiveRecordingPackets :one
-- /metrics に出す、録画中の録画のドロップ、エラー、スクランブルの合計。
SELECT COALESCE(sum(drops), 0)::bigint AS drops, COALESCE(sum(errors), 0)::bigint AS errors, COALESCE(sum(scrambled), 0)::bigint AS scrambled
FROM recordings WHERE state = 'recording';

-- name: CountRecordingAssignmentsByState :many
-- /metrics に出す、状態ごとの録画の割り振りの数。
SELECT state, count(*) AS count FROM recording_assignments GROUP BY state ORDER BY state;

-- name: CountTunersByState :one
-- /metrics に出す、状態ごとのチューナーの数。故障は、使えるかどうかより先に数える。
SELECT
  count(*) FILTER (WHERE NOT is_fault AND is_available) AS available,
  count(*) FILTER (WHERE NOT is_fault AND NOT is_available) AS unavailable,
  count(*) FILTER (WHERE is_fault) AS fault
FROM tuners;

-- name: HoldsLease :one
-- リース name を、holder が今持っているかどうか。
SELECT EXISTS (SELECT 1 FROM leases WHERE name = @name AND holder = @holder AND expires_at > @now);
