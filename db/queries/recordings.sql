-- name: StartRecording :one
-- 予約の録画を始める。引き継ぎや再開で、すでに録画があれば、それを続ける。新しく始めたかどうかを inserted で返す。
INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at, state, failure_reason,
  protected, drops, errors, scrambled, recorded_from, recorded_until, created_at, updated_at)
VALUES (@id, @reservation_id, @service_id, @event_id, @name, @description, @start_at, @end_at, 'recording', NULL,
  false, 0, 0, 0, @now, NULL, @now, @now)
ON CONFLICT (reservation_id) DO UPDATE SET state = 'recording', failure_reason = NULL, recorded_until = NULL, updated_at = EXCLUDED.updated_at
RETURNING sqlc.embed(recordings), (xmax = 0)::boolean AS inserted;

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

-- name: AddRecordingGap :exec
-- 途切れた区間を記録する。
UPDATE recordings SET gaps = gaps || jsonb_build_array(jsonb_build_object('from', @gap_from::timestamptz, 'to', @gap_to::timestamptz)), updated_at = @now
WHERE id = @id;

-- name: MarkRecordingProgramLost :exec
UPDATE recordings SET program_lost = true, updated_at = @now WHERE id = @id AND NOT program_lost;

-- name: GetLastRecordingFileEnd :one
-- 録画のセグメントのうち、最後に書き終えた時刻。引き継ぎや再開のときに、途切れた区間を求めるのに使う。
-- まだ書き終えたセグメントがなければ、行を返さない。何も書けなかったセグメントは、録れたものに数えない。
SELECT ended_at FROM recording_files
WHERE recording_id = @recording_id AND kind = 'segment' AND ended_at IS NOT NULL AND size > 0
ORDER BY ended_at DESC
LIMIT 1;

-- name: GetProgramEnd :one
-- 録画中の番組の、今の EPG の終了時刻と、未定かどうか。番組が EPG から消えていれば行を返さない。
SELECT end_at, end_undecided FROM programs WHERE service_id = @service_id AND event_id = @event_id;

-- name: InsertRecordingHistory :exec
-- 重複の回避に使う録画履歴を記録する (設計書 4.4)。録画を削除しても残す。
INSERT INTO recording_history (id, service_id, title_key, name, start_at, end_at, created_at)
VALUES (@id, @service_id, @title_key, @name, @start_at, @end_at, @now);

-- name: ListRecordingSegments :many
-- 通知に載せる、録画のセグメントの保存先プールとプール内の相対パス。何も書けなかったセグメントは含めない。
SELECT pool_id, path FROM recording_files
WHERE recording_id = @recording_id AND kind = 'segment' AND size > 0
ORDER BY started_at, id;

-- name: GetRecordingWithService :one
-- 通知に載せる、録画とサービス名。
SELECT sqlc.embed(recordings), s.name AS service_name
FROM recordings
JOIN services s ON s.id = recordings.service_id
WHERE recordings.id = @id;
