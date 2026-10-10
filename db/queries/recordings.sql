-- name: StartRecording :one
-- 予約の録画を始める。引き継ぎや再開で、すでに録画があれば、それを続ける。
INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at, state, failure_reason,
  protected, drops, errors, scrambled, recorded_from, recorded_until, created_at, updated_at)
VALUES (@id, @reservation_id, @service_id, @event_id, @name, @description, @start_at, @end_at, 'recording', NULL,
  false, 0, 0, 0, @now, NULL, @now, @now)
ON CONFLICT (reservation_id) DO UPDATE SET state = 'recording', failure_reason = NULL, recorded_until = NULL, updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: InsertRecordingFile :exec
INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
VALUES (@id, @recording_id, @kind, @pool_id, @path, 0, @started_at, NULL, @now);

-- name: CloseRecordingFile :exec
-- 書き終えたファイルの大きさと、表す時間の終わりを記録する。
UPDATE recording_files SET size = @size, ended_at = @ended_at WHERE id = @id;

-- name: AddRecordingStats :exec
-- 書き終えたセグメントのドロップ、エラー、スクランブルを、録画の集計に足す。
UPDATE recordings SET drops = drops + @drops, errors = errors + @errors, scrambled = scrambled + @scrambled, updated_at = @now
WHERE id = @id;

-- name: FinishRecording :exec
UPDATE recordings SET state = @state, failure_reason = sqlc.narg(failure_reason), recorded_until = @now, updated_at = @now
WHERE id = @id;

-- name: GetRecording :one
SELECT * FROM recordings WHERE id = @id;

-- name: ListRecordingFiles :many
SELECT * FROM recording_files WHERE recording_id = @recording_id ORDER BY started_at, kind, id;
