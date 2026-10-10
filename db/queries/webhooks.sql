-- name: InsertWebhook :one
INSERT INTO webhooks (id, name, url, format, kinds, secret_ciphertext, enabled, created_at, updated_at)
VALUES (@id, @name, @url, @format, @kinds, @secret_ciphertext, @enabled, @now, @now)
RETURNING *;

-- name: GetWebhook :one
SELECT * FROM webhooks WHERE id = @id;

-- name: LockWebhook :one
-- 変える前の値を監査ログに残すため、行をロックして読む。
SELECT * FROM webhooks WHERE id = @id FOR UPDATE;

-- name: ListWebhooks :many
SELECT * FROM webhooks ORDER BY created_at, id;

-- name: UpdateWebhook :one
UPDATE webhooks SET name = @name, url = @url, format = @format, kinds = @kinds, enabled = @enabled, updated_at = @now
WHERE id = @id
RETURNING *;

-- name: DeleteWebhook :execrows
DELETE FROM webhooks WHERE id = @id;

-- name: ListWebhooksForKind :many
-- イベントの種類 kind を送る、有効な Webhook の ID を返す。
SELECT id FROM webhooks WHERE enabled AND @kind::text = ANY(kinds) ORDER BY id;
