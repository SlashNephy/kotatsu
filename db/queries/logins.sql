-- name: GetUserByUsername :one
SELECT * FROM users WHERE lower(username) = lower(@username);

-- name: SetUserPassword :execrows
UPDATE users SET password_hash = @password_hash, updated_at = @now WHERE id = @id;

-- name: GetLoginThrottles :many
SELECT * FROM login_throttles WHERE key = ANY(@throttle_keys::text[]);

-- name: RecordLoginFailure :one
-- 失敗を数える。最後の失敗が reset_before より前なら、数え直す。
INSERT INTO login_throttles AS t (key, failures, locked_until, last_failed_at)
VALUES (@key, 1, @now, @now)
ON CONFLICT (key) DO UPDATE SET
  failures = CASE WHEN t.last_failed_at < @reset_before THEN 1 ELSE t.failures + 1 END,
  last_failed_at = @now
RETURNING failures;

-- name: SetLoginLockedUntil :exec
UPDATE login_throttles SET locked_until = @locked_until WHERE key = @key;

-- name: ResetLoginThrottle :exec
DELETE FROM login_throttles WHERE key = @key;

-- name: InsertLoginChallenge :exec
INSERT INTO login_challenges (id, token_hash, user_id, created_at, expires_at, user_agent, source_ip)
VALUES (@id, @token_hash, @user_id, @created_at, @expires_at, @user_agent, @source_ip);

-- name: DeleteExpiredLoginChallenges :execrows
DELETE FROM login_challenges WHERE expires_at <= @now;
