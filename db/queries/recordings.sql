-- name: StartRecording :one
-- 予約の録画を始める。引き継ぎや再開で、すでに録画があれば、それを続ける。新しく始めたかどうかを inserted で返す。
-- 番組の音声の構成は、始めたときの番組から写す。続けるときは、最初に写したものを残す。
INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at, state, failure_reason,
  protected, drops, errors, scrambled, audios, recorded_from, recorded_until, created_at, updated_at)
VALUES (@id, @reservation_id, @service_id, @event_id, @name, @description, @start_at, @end_at, 'recording', NULL,
  false, 0, 0, 0,
  COALESCE((SELECT p.audios FROM programs p WHERE p.service_id = @service_id AND p.event_id = sqlc.narg(event_id)), '[]'::jsonb),
  @now, NULL, @now, @now)
ON CONFLICT (reservation_id) DO UPDATE SET state = 'recording', failure_reason = NULL, recorded_until = NULL, updated_at = EXCLUDED.updated_at
RETURNING sqlc.embed(recordings), (xmax = 0)::boolean AS inserted;

-- name: InsertRecordingFile :exec
INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
VALUES (@id, @recording_id, @kind, @pool_id, @path, 0, @started_at, NULL, @now);

-- name: UpsertRecordingImage :exec
-- 録画の代表画像かプレビュー画像を記録する。作り直したときは、同じパスの行を書き換える。
INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
VALUES (@id, @recording_id, @kind, @pool_id, @path, @size, @started_at, @now, @now)
ON CONFLICT (pool_id, path) DO UPDATE SET size = EXCLUDED.size, ended_at = EXCLUDED.ended_at
WHERE recording_files.recording_id = EXCLUDED.recording_id AND recording_files.kind = EXCLUDED.kind;

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

-- name: MarkRecordingProgramLost :exec
UPDATE recordings SET program_lost = true, updated_at = @now WHERE id = @id AND NOT program_lost;

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

-- name: ListOpenRecordingFiles :many
-- 録画の、書き終えていないファイル。落ちた mikan が書きかけたまま残したものを、引き継ぐ mikan が閉じる。
SELECT * FROM recording_files WHERE recording_id = @recording_id AND ended_at IS NULL ORDER BY started_at, id;

-- name: SetRecordingGaps :exec
-- 録画を終えるときに、セグメントの時間の範囲から求めた途切れた区間を記録する。
UPDATE recordings SET gaps = @gaps, updated_at = @now WHERE id = @id;

-- name: UpsertRecordingIndex :exec
-- 録画を終えた後に作ったセグメントの索引を記録する。索引は、セグメントと同じ時間の範囲を表す。作り直したときは、同じパスの行を書き換える。
INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
VALUES (@id, @recording_id, 'index', @pool_id, @path, @size, @started_at, sqlc.narg(ended_at), @now)
ON CONFLICT (pool_id, path) DO UPDATE SET size = EXCLUDED.size, ended_at = EXCLUDED.ended_at
WHERE recording_files.recording_id = EXCLUDED.recording_id AND recording_files.kind = 'index';

-- name: SetRecordingStats :exec
-- 録画を終えた後に解析したドロップ、エラー、スクランブルで、録画の集計を置き換える。
-- 完了した録画は、ドロップの有無で、完了とエラーありの完了を決め直す。
UPDATE recordings SET drops = @drops, errors = @errors, scrambled = @scrambled,
  state = CASE WHEN state IN ('completed', 'completed_with_errors')
    THEN CASE WHEN @drops::bigint > 0 THEN 'completed_with_errors' ELSE 'completed' END
    ELSE state END,
  updated_at = @now
WHERE id = @id;

-- name: GetRecordingJikkyoSource :one
-- 実況の過去ログを引くための、録画した時間帯とサービス (設計書 7.4)。
SELECT r.recorded_from, r.recorded_until, s.network_id, s.sid
FROM recordings r
JOIN services s ON s.id = r.service_id
WHERE r.id = @id;

-- name: SetRecordingAudiosIfUnknown :exec
-- 録画の音声の構成を、TS の EIT[p/f] から読んだもので埋める。EPG から写したものがあれば、それを残す。
UPDATE recordings SET audios = @audios, updated_at = @now WHERE id = @id AND audios = '[]'::jsonb;

-- name: DeleteRecordingFiles :exec
-- 録画のファイルの記録を消す。セグメントを 1 つにつないだあとで、つないだ元のセグメントと索引の記録を消すのに使う。
DELETE FROM recording_files WHERE recording_id = @recording_id AND id = ANY(@ids::uuid[]);
