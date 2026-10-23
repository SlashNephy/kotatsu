-- name: CreateAPIToken :exec
INSERT INTO api_tokens (id, user_id, name, token_hash, created_at, last_used_at)
VALUES (@id, @user_id, @name, @token_hash, @now, NULL);

-- name: ListAPITokens :many
-- ユーザーの API トークンを、新しい順に返す。
SELECT id, name, created_at, last_used_at FROM api_tokens WHERE user_id = @user_id ORDER BY created_at DESC, id;

-- name: DeleteAPIToken :one
-- ユーザーの API トークンを消し、消した名前を返す。ほかのユーザーのトークンは消さない。
DELETE FROM api_tokens WHERE id = @id AND user_id = @user_id RETURNING name;

-- name: GetAPITokenUser :one
-- トークンのハッシュから、発行したユーザーを引く。無効にしたユーザーのトークンは使えない。
SELECT t.id, t.last_used_at, u.id AS user_id, u.role
FROM api_tokens t
JOIN users u ON u.id = t.user_id
WHERE t.token_hash = @token_hash AND u.disabled_at IS NULL;

-- name: TouchAPIToken :exec
UPDATE api_tokens SET last_used_at = @now WHERE id = @id;
