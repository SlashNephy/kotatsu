-- name: ListTuners :many
SELECT tuner_index, name, types, is_available, is_remote, is_fault FROM tuners ORDER BY tuner_index;

-- name: DeleteTuners :exec
DELETE FROM tuners;

-- name: InsertTuner :exec
INSERT INTO tuners (tuner_index, name, types, is_available, is_remote, is_fault, updated_at)
VALUES (@tuner_index, @name, @types, @is_available, @is_remote, @is_fault, @now);

-- name: GetTunerServerStatus :one
SELECT degraded_since, last_error, clock_skew_ms, checked_at, broadcast_clock_skew_ms, broadcast_clock_checked_at FROM tuner_server_status WHERE id;

-- name: UpsertTunerServerStatus :exec
INSERT INTO tuner_server_status (id, degraded_since, last_error, clock_skew_ms, checked_at)
VALUES (true, @degraded_since, @last_error, @clock_skew_ms, @checked_at)
ON CONFLICT (id) DO UPDATE SET
  degraded_since = excluded.degraded_since,
  last_error = excluded.last_error,
  clock_skew_ms = excluded.clock_skew_ms,
  checked_at = excluded.checked_at;

-- name: SetBroadcastClockSkew :exec
-- 録画している TS の TOT で測った、放送の時刻と OS の時計の差を書く (#1309)。行がなければ作る。
INSERT INTO tuner_server_status (id, degraded_since, last_error, clock_skew_ms, checked_at, broadcast_clock_skew_ms, broadcast_clock_checked_at)
VALUES (true, NULL, '', NULL, @now, @skew_ms, @now)
ON CONFLICT (id) DO UPDATE SET broadcast_clock_skew_ms = excluded.broadcast_clock_skew_ms, broadcast_clock_checked_at = excluded.broadcast_clock_checked_at;
