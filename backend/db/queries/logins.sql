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

-- name: DeleteStaleLoginThrottles :execrows
-- 数え直す時間を過ぎ、待たせてもいない行を消す。存在しないユーザー名や IP の行が溜まり続けないようにする。
DELETE FROM login_throttles WHERE last_failed_at < @reset_before AND locked_until <= @now;

-- name: GetLoginChallengeForUpdate :one
SELECT * FROM login_challenges WHERE token_hash = @token_hash AND expires_at > @now FOR UPDATE;

-- name: IncrementLoginChallengeFailures :one
UPDATE login_challenges SET failed_attempts = failed_attempts + 1 WHERE id = @id RETURNING failed_attempts;

-- name: DeleteLoginChallenge :exec
DELETE FROM login_challenges WHERE id = @id;

-- name: LockUser :one
-- ユーザーの行を、トランザクションが終わるまで書き込みからロックする。同じユーザーの登録を同時に行わせない。
SELECT id, username FROM users WHERE id = @id FOR UPDATE;

-- name: DeleteUserLoginChallenges :exec
-- ユーザーの、第二要素の確認を待つログインをすべて消す。パスワードを設定したときと、無効にしたときに使う。
DELETE FROM login_challenges WHERE user_id = @user_id;

-- name: TryLockLoginKey :one
-- 試行回数の制限のキーを、トランザクションの終わりまでロックする。ほかの試行がロックしていれば、待たずに false を返す。
-- 同じアカウントか IP の試行を 1 つずつ確かめ、同時に送った試行が、前の失敗を見ずに待ち時間を素通りしないようにする。
SELECT pg_try_advisory_xact_lock(hashtext('kotatsu.login'), hashtext(@key::text))::boolean AS locked;
