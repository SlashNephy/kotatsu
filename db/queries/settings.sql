-- name: GetSystemSettings :one
SELECT * FROM system_settings WHERE id = 1;

-- name: LockSystemSettings :one
-- 行がなければ既定値で作り、行をロックする。同時に変えても、片方の変更を失わない。
INSERT INTO system_settings (id, recording_defaults, archive_retention_days, updated_at)
VALUES (1, '{}', @archive_retention_days, @now)
ON CONFLICT (id) DO UPDATE SET id = system_settings.id
RETURNING *;

-- name: UpdateSystemSettings :one
UPDATE system_settings SET recording_defaults = @recording_defaults, archive_retention_days = @archive_retention_days, updated_at = @now
WHERE id = 1
RETURNING *;
