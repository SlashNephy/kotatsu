-- name: GetSystemSettings :one
SELECT * FROM system_settings WHERE id = 1;

-- name: LockSystemSettings :one
-- 行がなければ既定値で作り、行をロックする。同時に変えても、片方の変更を失わない。
INSERT INTO system_settings (id, recording_defaults, archive_retention_days, updated_at)
VALUES (1, '{}', @archive_retention_days, @now)
ON CONFLICT (id) DO UPDATE SET id = system_settings.id
RETURNING *;

-- name: UpdateSystemSettings :one
UPDATE system_settings SET recording_defaults = @recording_defaults, archive_retention_days = @archive_retention_days,
  notification_retention_days = @notification_retention_days, rule_defaults = @rule_defaults, mcp_enabled = @mcp_enabled,
  data_broadcast_ip_enabled = @data_broadcast_ip_enabled, drcs_replacements = @drcs_replacements, updated_at = @now
WHERE id = 1
RETURNING *;

-- name: LockChannelType :one
-- 変える前の値を監査ログに残すため、種別の行をロックして読む。
SELECT * FROM channel_types WHERE type = @type FOR UPDATE;

-- name: UpdateChannelType :one
UPDATE channel_types SET display_name = @display_name, sort_order = @sort_order WHERE type = @type
RETURNING *;

-- name: LockService :one
-- 変える前の値を監査ログに残すため、サービスの行をロックして読む。
SELECT id, hidden, sort_order FROM services WHERE id = @id FOR UPDATE;

-- name: UpdateServiceDisplay :exec
-- 番組表に出さない印と、並び順の上書きを変える。EPG の同期は、この 2 つを上書きしない (設計書 8.2)。
UPDATE services SET hidden = @hidden, sort_order = sqlc.narg(sort_order), updated_at = @now WHERE id = @id;
