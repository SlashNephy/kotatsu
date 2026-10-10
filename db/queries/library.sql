-- name: LockRecording :one
-- 録画を変える前に、ロックして読む。
SELECT * FROM recordings WHERE id = @id FOR UPDATE;

-- name: SetRecordingProtected :exec
UPDATE recordings SET protected = @protected, updated_at = @now WHERE id = @id;

-- name: UpsertTag :one
-- 名前のタグを作り、ID を返す。すでにあれば、そのタグの ID を返す。
INSERT INTO tags (id, name, created_at) VALUES (@id, @name, @now)
ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
RETURNING id;

-- name: DeleteRecordingTags :exec
DELETE FROM recording_tags WHERE recording_id = @recording_id;

-- name: InsertRecordingTag :exec
INSERT INTO recording_tags (recording_id, tag_id) VALUES (@recording_id, @tag_id) ON CONFLICT DO NOTHING;

-- name: ListRecordingTags :many
-- 録画 recording_ids のタグを、録画ごとに名前の順で返す。
SELECT rt.recording_id, t.id, t.name FROM recording_tags rt
JOIN tags t ON t.id = rt.tag_id
WHERE rt.recording_id = ANY(@recording_ids::uuid[])
ORDER BY rt.recording_id, t.name;

-- name: ListTags :many
-- タグを、付いている録画の数と一緒に、名前の順で返す。
SELECT t.id, t.name, count(rt.recording_id)::bigint AS recordings FROM tags t
LEFT JOIN recording_tags rt ON rt.tag_id = t.id
GROUP BY t.id, t.name
ORDER BY t.name;

-- name: UpsertWatchProgress :exec
INSERT INTO watch_progress (user_id, recording_id, position_ms, updated_at) VALUES (@user_id, @recording_id, @position_ms, @now)
ON CONFLICT (user_id, recording_id) DO UPDATE SET position_ms = EXCLUDED.position_ms, updated_at = EXCLUDED.updated_at;

-- name: ListWatchProgress :many
SELECT * FROM watch_progress WHERE user_id = @user_id AND recording_id = ANY(@recording_ids::uuid[]);

-- name: SearchRecordings :many
-- 録画を、録画を始めた日時の新しい順に返す。絞り込まない条件は NULL か空にする。
-- before_at と before_id より前 (古い) の録画だけを返し、ページを送る。2 つは、両方を渡すか、両方を NULL にする。
SELECT r.* FROM recordings r
WHERE (cardinality(@states::text[]) = 0 OR r.state = ANY(@states::text[]))
  AND (cardinality(@service_ids::bigint[]) = 0 OR r.service_id = ANY(@service_ids::bigint[]))
  AND (cardinality(@tag_ids::uuid[]) = 0 OR EXISTS (
    SELECT 1 FROM recording_tags rt WHERE rt.recording_id = r.id AND rt.tag_id = ANY(@tag_ids::uuid[])
  ))
  AND (sqlc.narg(keyword)::text IS NULL OR r.name ILIKE '%' || sqlc.narg(keyword)::text || '%')
  AND (sqlc.narg(since)::timestamptz IS NULL OR r.start_at >= sqlc.narg(since)::timestamptz)
  AND (sqlc.narg(until)::timestamptz IS NULL OR r.start_at < sqlc.narg(until)::timestamptz)
  AND (sqlc.narg(before_at)::timestamptz IS NULL
    OR (r.recorded_from, r.id) < (sqlc.narg(before_at)::timestamptz, sqlc.narg(before_id)::uuid))
ORDER BY r.recorded_from DESC, r.id DESC
LIMIT @max_count;

-- name: ListRecordingsByIDs :many
SELECT * FROM recordings WHERE id = ANY(@ids::uuid[]);

-- name: ListRecordingFilesByRecordings :many
SELECT * FROM recording_files WHERE recording_id = ANY(@recording_ids::uuid[]) ORDER BY recording_id, started_at, kind, id;
