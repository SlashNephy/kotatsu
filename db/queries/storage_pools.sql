-- name: ListStoragePools :many
SELECT * FROM storage_pools ORDER BY id;

-- name: UpsertStoragePool :one
INSERT INTO storage_pools (id, priority, min_free_bytes, low_since, updated_at)
VALUES (@id, @priority, @min_free_bytes, NULL, @now)
ON CONFLICT (id) DO UPDATE SET priority = EXCLUDED.priority, min_free_bytes = EXCLUDED.min_free_bytes, updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: MarkStoragePoolLow :execrows
-- 空き容量が閾値を下回ったままになったことを記録する。すでに記録していれば何もしない (0 行)。
UPDATE storage_pools SET low_since = @now WHERE id = @id AND low_since IS NULL;

-- name: ClearStoragePoolLow :exec
UPDATE storage_pools SET low_since = NULL WHERE id = @id AND low_since IS NOT NULL;

-- name: ListDeletableRecordings :many
-- 容量の管理で消せる録画を、古い順に返す。プール pool_id にファイルを持ち、保護されておらず、録画中でなく、
-- すべてのファイルがこのインスタンスにマウントされたプール mounted にあるもの。
SELECT r.id, r.name FROM recordings r
WHERE NOT r.protected AND r.state <> 'recording'
  AND EXISTS (SELECT 1 FROM recording_files f WHERE f.recording_id = r.id AND f.pool_id = @pool_id)
  AND NOT EXISTS (SELECT 1 FROM recording_files f WHERE f.recording_id = r.id AND NOT (f.pool_id = ANY(@mounted::text[])))
ORDER BY r.recorded_from, r.id
LIMIT @max_count;

-- name: LockDeletableRecording :one
-- 消す直前に、録画をロックして、まだ消せることを確かめる。
SELECT id, name FROM recordings WHERE id = @id AND NOT protected AND state <> 'recording' FOR UPDATE;

-- name: DeleteRecording :execrows
DELETE FROM recordings WHERE id = @id;

-- name: LockStoragePool :one
-- 変える前の値を監査ログに残すため、行をロックして読む。行がなければ、既定値で作ってから読む。
INSERT INTO storage_pools (id, priority, min_free_bytes, low_since, updated_at)
VALUES (@id, 0, 0, NULL, @now)
ON CONFLICT (id) DO UPDATE SET id = EXCLUDED.id
RETURNING *;
