-- name: ListTuners :many
SELECT tuner_index, name, types, is_available, is_remote, is_fault FROM tuners ORDER BY tuner_index;

-- name: DeleteTuners :exec
DELETE FROM tuners;

-- name: InsertTuner :exec
INSERT INTO tuners (tuner_index, name, types, is_available, is_remote, is_fault, updated_at)
VALUES (@tuner_index, @name, @types, @is_available, @is_remote, @is_fault, @now);

-- name: GetTunerServerStatus :one
SELECT degraded_since, last_error, clock_skew_ms, checked_at, broadcast_clock_skew_ms, broadcast_clock_checked_at, server_kind, server_version FROM tuner_server_status WHERE id;

-- name: UpsertTunerServerStatus :exec
-- 種類と版が NULL なら (接続できない、判定に失敗した)、最後に分かったものを残す (#1538)。
INSERT INTO tuner_server_status (id, degraded_since, last_error, clock_skew_ms, checked_at, server_kind, server_version)
VALUES (true, @degraded_since, @last_error, @clock_skew_ms, @checked_at, sqlc.narg(server_kind), sqlc.narg(server_version))
ON CONFLICT (id) DO UPDATE SET
  degraded_since = excluded.degraded_since,
  last_error = excluded.last_error,
  clock_skew_ms = excluded.clock_skew_ms,
  checked_at = excluded.checked_at,
  server_kind = COALESCE(excluded.server_kind, tuner_server_status.server_kind),
  server_version = COALESCE(excluded.server_version, tuner_server_status.server_version);

-- name: SetBroadcastClockSkew :exec
-- 録画している TS の TOT で測った、放送の時刻と OS の時計の差を書く (#1309)。行がなければ作る。
INSERT INTO tuner_server_status (id, degraded_since, last_error, clock_skew_ms, checked_at, broadcast_clock_skew_ms, broadcast_clock_checked_at)
VALUES (true, NULL, '', NULL, @now, @skew_ms, @now)
ON CONFLICT (id) DO UPDATE SET broadcast_clock_skew_ms = excluded.broadcast_clock_skew_ms, broadcast_clock_checked_at = excluded.broadcast_clock_checked_at;
