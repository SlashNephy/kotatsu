-- name: GetBackupSettings :one
SELECT recipients, interval_hours, generations, updated_at FROM backup_settings WHERE id = 1;

-- name: SaveBackupSettings :exec
INSERT INTO backup_settings (id, recipients, interval_hours, generations, updated_at)
VALUES (1, @recipients, @interval_hours, @generations, @now)
ON CONFLICT (id) DO UPDATE SET
  recipients = EXCLUDED.recipients, interval_hours = EXCLUDED.interval_hours, generations = EXCLUDED.generations, updated_at = EXCLUDED.updated_at;

-- name: InsertBackupRun :exec
INSERT INTO backup_runs (id, trigger, started_at, finished_at, file_name, size_bytes, error)
VALUES (@id, @trigger, @started_at, @finished_at, sqlc.narg(file_name), sqlc.narg(size_bytes), sqlc.narg(error));

-- name: GetLatestBackupRun :one
SELECT id, trigger, started_at, finished_at, file_name, size_bytes, error FROM backup_runs ORDER BY started_at DESC, id DESC LIMIT 1;

-- name: PruneBackupRuns :exec
-- 新しいものから keep 件を残し、古い結果を消す。
DELETE FROM backup_runs WHERE id NOT IN (SELECT id FROM backup_runs ORDER BY started_at DESC, id DESC LIMIT @keep::integer);
