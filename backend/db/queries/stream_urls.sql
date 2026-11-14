-- name: CreateStreamURL :exec
INSERT INTO stream_urls (id, user_id, resource, target_name, created_at, expires_at, last_used_at)
VALUES (@id, @user_id, @resource, @target_name, @now, @expires_at, NULL);

-- name: DeleteExpiredStreamURLs :exec
-- 期限の過ぎた URL の記録を消す。発行するときに呼ぶ。
DELETE FROM stream_urls WHERE expires_at <= @now;

-- name: ListStreamURLs :many
-- 期限の過ぎていない URL の記録を、新しい順に、発行したユーザーと一緒に返す。
SELECT s.id, s.resource, s.target_name, s.created_at, s.expires_at, s.last_used_at, u.id AS user_id, u.display_name
FROM stream_urls s
JOIN users u ON u.id = s.user_id
WHERE s.expires_at > @now
ORDER BY s.created_at DESC, s.id;

-- name: DeleteStreamURL :one
-- URL の記録を消し、監査ログに残すため、発行したユーザーと対象の名前を返す。
DELETE FROM stream_urls WHERE id = @id RETURNING user_id, target_name;

-- name: GetStreamURLUser :one
-- URL の記録から、発行したユーザーを引く。無効にしたユーザーの URL は使えない。
SELECT s.user_id, s.last_used_at, u.role
FROM stream_urls s
JOIN users u ON u.id = s.user_id
WHERE s.id = @id AND u.disabled_at IS NULL;

-- name: TouchStreamURL :exec
UPDATE stream_urls SET last_used_at = @now WHERE id = @id;
