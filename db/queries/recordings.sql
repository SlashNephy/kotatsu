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

-- name: SetJoinedRecordingFile :execrows
-- セグメントを 1 つにつないだあとで、最初のセグメントの大きさと、表す時間の終わりを、つないだものにする。
-- 録画が消されて行がなければ 0 を返す。
UPDATE recording_files SET size = @size, ended_at = @ended_at WHERE id = @id;

-- name: CountRecordingSegments :one
-- 録画のセグメントの記録の数と、そのうち最初のもの first_id の大きさ。つないだ結果が確定したかを確かめるのに使う。
SELECT count(*) AS segments, coalesce(max(size) FILTER (WHERE id = @first_id), 0)::bigint AS first_size
FROM recording_files WHERE recording_id = @recording_id AND kind = 'segment' AND size > 0;

-- name: DeleteRecordingFiles :exec
-- 録画のファイルの記録を消す。セグメントを 1 つにつないだあとで、つないだ元のセグメントと索引の記録を消すのに使う。
DELETE FROM recording_files WHERE recording_id = @recording_id AND id = ANY(@ids::uuid[]);

-- name: ListFinishedRecordingSegments :many
-- 録画中でない録画のセグメントの記録を、録画を始めた日時の新しい順に返す。ファイルの見つからない録画を探すのに使う。
SELECT r.id AS recording_id, f.id, f.pool_id, f.path
FROM recordings r JOIN recording_files f ON f.recording_id = r.id AND f.kind = 'segment'
WHERE r.state <> 'recording'
ORDER BY r.recorded_from DESC, r.id, f.started_at, f.id;
-- name: ListRecordingsForRename :many
-- ファイル名を付け直す録画と、ファイル名のテンプレートに当てはめる番組とサービスと、録画設定を求めるための予約。
-- 録画中の録画は除く。予約が消えていれば、予約の項目は NULL になる。
SELECT r.id, r.name, r.start_at, r.end_at, r.event_id, s.name AS service_name, s.channel_type, s.channel,
  res.id AS reservation_id, res.kind AS reservation_kind, res.settings AS reservation_settings
FROM recordings r
JOIN services s ON s.id = r.service_id
LEFT JOIN reservations res ON res.id = r.reservation_id
WHERE r.state <> 'recording' AND (sqlc.narg(ids)::uuid[] IS NULL OR r.id = ANY(sqlc.narg(ids)::uuid[]))
ORDER BY r.recorded_from DESC, r.id;

-- name: SetRecordingFilePath :exec
UPDATE recording_files SET path = @path WHERE id = @id;

-- name: MoveRecordingSidecarPaths :exec
-- 付属ファイルのディレクトリを移したとき、その中のファイル (索引、代表画像、プレビュー画像) の記録のパスを書き換える。
UPDATE recording_files SET path = @new_prefix::text || substr(path, length(@old_prefix::text) + 1)
WHERE recording_id = @recording_id AND pool_id = @pool_id AND kind IN ('index', 'thumbnail', 'preview')
  AND starts_with(path, @old_prefix::text);

-- name: SetRecordingBroadcastStarted :exec
-- 予約した番組が放送中になった日時を、最初の一度だけ記録する。
UPDATE recordings SET broadcast_started_at = @at, updated_at = @at WHERE id = @id AND broadcast_started_at IS NULL;

-- name: SetRecordingBroadcastEnded :exec
-- 予約した番組が放送中でなくなった日時を、最初の一度だけ記録する。
UPDATE recordings SET broadcast_ended_at = @at, updated_at = @at WHERE id = @id AND broadcast_ended_at IS NULL;

-- name: MarkRecordingNotBroadcast :exec
-- 予約した番組が放送されなかったことと、その時間に放送されていた番組を記録する。
UPDATE recordings SET not_broadcast = true, aired_event_id = sqlc.narg(aired_event_id), updated_at = @now WHERE id = @id;

-- name: GetAiredProgramName :one
-- 予約した番組が放送されなかったときに、その時間に放送されていた番組の名前。番組表か、アーカイブから引く。
-- eventId は日をまたいで使い回されるので、録画の予定の時間 (window_start〜window_end) に重なる番組に絞る。
SELECT name FROM (
  SELECT p.name, p.start_at FROM programs p
  WHERE p.service_id = @service_id AND p.event_id = @event_id AND p.start_at < @window_end::timestamptz AND p.end_at > @window_start::timestamptz
  UNION ALL
  SELECT a.name, a.start_at FROM program_archives a
  WHERE a.service_id = @service_id AND a.event_id = @event_id AND a.start_at < @window_end::timestamptz AND a.end_at > @window_start::timestamptz
) candidates
ORDER BY abs(extract(epoch FROM start_at - @window_start::timestamptz))
LIMIT 1;

-- name: TryLockRecordingJoin :one
-- 録画のセグメントをつなぐ間、ほかの nekonote が同じ録画をつながないよう、セッションの advisory lock を取る。取れなければ false を返す。
SELECT pg_try_advisory_lock(hashtext('kotatsu.segjoin'), hashtext(@recording_id::text)) AS locked;

-- name: UnlockRecordingJoin :one
SELECT pg_advisory_unlock(hashtext('kotatsu.segjoin'), hashtext(@recording_id::text)) AS unlocked;

-- name: GetStoragePoolThreshold :one
-- プールの容量の閾値。設定していなければ 0 を返す。
SELECT coalesce((SELECT min_free_bytes FROM storage_pools WHERE id = @id), 0)::bigint AS min_free_bytes;

-- name: FailStuckRecordings :many
-- 番組の終わりが before より前に過ぎても「録画中」のままで、録画している割り振り (終わっていないもの) もない録画を、失敗にする。
-- 録画していた mikan がいなくなったか、予約を消して割り振りが消えたもので、ほかに録画を終える人がいない (#516)。
-- 録画した時間の終わりは、書いたセグメントの終わりにする。
UPDATE recordings r SET
  state = 'failed',
  failure_reason = 'stream_interrupted',
  recorded_until = coalesce(
    (SELECT max(coalesce(f.ended_at, f.started_at)) FROM recording_files f WHERE f.recording_id = r.id AND f.kind = 'segment'),
    r.recorded_from
  ),
  updated_at = @now
FROM services s
WHERE s.id = r.service_id
  AND r.state = 'recording'
  AND r.end_at < @before::timestamptz
  AND NOT EXISTS (
    SELECT 1 FROM recording_assignments a WHERE a.reservation_id = r.reservation_id AND a.state <> 'finished'
  )
RETURNING r.id, r.reservation_id, r.name, r.start_at, r.end_at, s.name AS service_name;

-- name: InsertMissedRecording :execrows
-- mikan が取らなかった予約の録画を、失敗として記録する (#517)。録画した時間はないので、始まりと終わりを予定の開始にする。
-- 同じ予約の録画がすでにあれば何もせず、0 を返す。
INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, start_at, end_at, state, failure_reason,
  recorded_from, recorded_until, created_at, updated_at)
VALUES (@id, @reservation_id, @service_id, @event_id, @name, @description, @start_at, @end_at, 'failed', 'recorder_unavailable',
  @recorded_from, @recorded_from, @now, @now)
ON CONFLICT (reservation_id) DO NOTHING;
