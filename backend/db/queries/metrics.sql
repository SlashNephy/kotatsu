-- name: CountRecordingsByState :many
-- /metrics に出す、状態ごとの録画の数 (設計書 11.3)。
SELECT state, count(*) AS count FROM recordings GROUP BY state ORDER BY state;

-- name: CountRecordingFailures :many
-- /metrics に出す、失敗の理由ごとの録画の数。
SELECT failure_reason::text AS reason, count(*) AS count FROM recordings WHERE failure_reason IS NOT NULL GROUP BY failure_reason ORDER BY failure_reason;

-- name: ListRecordingFailureCounts :many
-- /metrics に出す、失敗の理由ごとの、録画の失敗の累計と最後に失敗した日時 (#960)。録画を消しても減らない。
SELECT reason, count, last_failed_at FROM recording_failure_counts ORDER BY reason;

-- name: SumActiveRecordingPackets :one
-- /metrics に出す、録画中の録画のドロップ、エラー、スクランブルの合計。閉じたセグメントの数に、書いているセグメントの数 (#829) を足す。
SELECT
  (COALESCE(sum(r.drops), 0) + COALESCE(sum(l.drops), 0))::bigint AS drops,
  (COALESCE(sum(r.errors), 0) + COALESCE(sum(l.errors), 0))::bigint AS errors,
  (COALESCE(sum(r.scrambled), 0) + COALESCE(sum(l.scrambled), 0))::bigint AS scrambled
FROM recordings r LEFT JOIN recording_live_stats l ON l.recording_id = r.id
WHERE r.state = 'recording';

-- name: SumActiveRecordingBytes :one
-- /metrics に出す、録画中の録画が書いたバイト数の合計 (#829)。閉じたセグメントの大きさに、書いているセグメントのバイト数を足す。
SELECT (
  COALESCE((SELECT sum(f.size) FROM recording_files f JOIN recordings r ON r.id = f.recording_id
    WHERE r.state = 'recording' AND f.kind = 'segment' AND f.ended_at IS NOT NULL), 0)
  + COALESCE((SELECT sum(l.bytes) FROM recording_live_stats l JOIN recordings r ON r.id = l.recording_id
    WHERE r.state = 'recording'), 0)
)::bigint AS bytes;

-- name: CountReservationsByState :one
-- /metrics に出す、録画の予定の終わりが来ていない予約の、割り当ての状態ごとの数。無効な予約は、割り当ての状態によらず無効として数える。
-- 終わったかどうかは、割り当ての対象 (ListAssignableReservations) と同じく、予定の終わりで決める。
SELECT
  count(*) FILTER (WHERE enabled AND assignment_state = 'ok') AS ok,
  count(*) FILTER (WHERE enabled AND assignment_state = 'partial') AS partial,
  count(*) FILTER (WHERE enabled AND assignment_state = 'conflict') AS conflict,
  count(*) FILTER (WHERE enabled AND assignment_state = 'pending') AS pending,
  count(*) FILTER (WHERE NOT enabled) AS disabled
FROM reservations
WHERE coalesce(planned_end_at, end_at) > @now::timestamptz;

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

-- name: ListStoragePoolMeasurements :many
-- /metrics に出す、容量の管理が測った保存先プールの容量と空き容量、閾値 (#957)。測っていないプールは返さない。
SELECT id, min_free_bytes, total_bytes::bigint AS total_bytes, free_bytes::bigint AS free_bytes, measured_at::timestamptz AS measured_at
FROM storage_pools WHERE measured_at IS NOT NULL ORDER BY id;

-- name: HoldsLease :one
-- リース name を、holder が今持っているかどうか。
SELECT EXISTS (SELECT 1 FROM leases WHERE name = @name AND holder = @holder AND expires_at > @now);
