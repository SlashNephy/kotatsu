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
