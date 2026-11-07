-- name: UpsertCalendarFeed :exec
-- ユーザーの、種類 kind の URL を発行する。すでにあれば、秘密の値を差し替え、前の URL を使えなくする。
INSERT INTO calendar_feeds (user_id, kind, token_hash, created_at, last_used_at) VALUES (@user_id, @kind, @token_hash, @now, NULL)
ON CONFLICT (user_id, kind) DO UPDATE SET token_hash = EXCLUDED.token_hash, created_at = EXCLUDED.created_at, last_used_at = NULL;

-- name: GetCalendarFeed :one
SELECT * FROM calendar_feeds WHERE user_id = @user_id AND kind = @kind;

-- name: DeleteCalendarFeed :execrows
DELETE FROM calendar_feeds WHERE user_id = @user_id AND kind = @kind;

-- name: GetCalendarFeedUser :one
-- 種類 kind の秘密の値のハッシュから、発行したユーザーを引く。無効にしたユーザーの URL は使えない。
-- ほかの種類の値は、同じユーザーのものでも受け付けない。
SELECT f.last_used_at, u.id AS user_id, u.role
FROM calendar_feeds f
JOIN users u ON u.id = f.user_id
WHERE f.token_hash = @token_hash AND f.kind = @kind AND u.disabled_at IS NULL;

-- name: TouchCalendarFeed :exec
UPDATE calendar_feeds SET last_used_at = @now WHERE user_id = @user_id AND kind = @kind;
