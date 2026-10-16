-- name: InsertSession :exec
INSERT INTO sessions (
  id, token_hash, user_id, created_at, authenticated_at, last_seen_at, expires_at, revoked_at, user_agent, source_ip
) VALUES (
  @id, @token_hash, @user_id, @created_at, @created_at, @created_at, @expires_at, NULL, @user_agent, @source_ip
);

-- name: GetSessionByTokenHash :one
SELECT sqlc.embed(sessions), users.role, users.disabled_at AS user_disabled_at
FROM sessions
JOIN users ON users.id = sessions.user_id
WHERE sessions.token_hash = @token_hash;

-- name: TouchSession :exec
-- 最後に使われた日時を更新する。stale_before より前に更新したセッションだけを書き換え、書き込みを減らす。
UPDATE sessions SET last_seen_at = @now WHERE id = @id AND last_seen_at < @stale_before;

-- name: ListActiveSessions :many
SELECT * FROM sessions
WHERE user_id = @user_id AND revoked_at IS NULL AND expires_at > @now AND last_seen_at > @idle_since
ORDER BY last_seen_at DESC, id DESC;

-- name: RevokeSession :execrows
UPDATE sessions SET revoked_at = @now::timestamptz WHERE id = @id AND user_id = @user_id AND revoked_at IS NULL;

-- name: ReauthenticateSession :execrows
-- 再認証した日時を記録する (設計書 9.5)。失効したセッションは更新しない。
UPDATE sessions SET authenticated_at = @now WHERE id = @id AND user_id = @user_id AND revoked_at IS NULL;

-- name: DeleteStaleSessions :execrows
-- 使えなくなったセッション (失効させた、絶対的な期限を過ぎた、無操作の期限を過ぎた) を、max_count 件まで消す。
DELETE FROM sessions WHERE id IN (
  SELECT s.id FROM sessions s
  WHERE s.revoked_at IS NOT NULL OR s.expires_at <= @now OR s.last_seen_at <= @idle_since
  LIMIT @max_count
);
