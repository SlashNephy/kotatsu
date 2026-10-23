-- name: ListTuners :many
SELECT tuner_index, name, types, is_available, is_remote, is_fault FROM tuners ORDER BY tuner_index;

-- name: DeleteTuners :exec
DELETE FROM tuners;

-- name: InsertTuner :exec
INSERT INTO tuners (tuner_index, name, types, is_available, is_remote, is_fault, updated_at)
VALUES (@tuner_index, @name, @types, @is_available, @is_remote, @is_fault, @now);

-- name: GetTunerServerStatus :one
SELECT degraded_since, last_error, clock_skew_ms, checked_at FROM tuner_server_status WHERE id;

-- name: UpsertTunerServerStatus :exec
INSERT INTO tuner_server_status (id, degraded_since, last_error, clock_skew_ms, checked_at)
VALUES (true, @degraded_since, @last_error, @clock_skew_ms, @checked_at)
ON CONFLICT (id) DO UPDATE SET
  degraded_since = excluded.degraded_since,
  last_error = excluded.last_error,
  clock_skew_ms = excluded.clock_skew_ms,
  checked_at = excluded.checked_at;
