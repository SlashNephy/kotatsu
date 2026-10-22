-- name: CreateOAuthAuthorizationCode :exec
INSERT INTO oauth_authorization_codes (
  code_hash, user_id, client_id, client_name, redirect_uri, code_challenge, scopes, resource, created_at, expires_at
) VALUES (
  @code_hash, @user_id, @client_id, @client_name, @redirect_uri, @code_challenge, @scopes, @resource, @now, @expires_at
);

-- name: ConsumeOAuthAuthorizationCode :one
-- 認可コードを消して返す。一度しか引き換えられないよう、引き換えるときに消す。期限を過ぎたものは返さない。
DELETE FROM oauth_authorization_codes c
USING users u
WHERE c.code_hash = @code_hash AND c.expires_at > @now AND u.id = c.user_id AND u.disabled_at IS NULL
RETURNING c.user_id, c.client_id, c.client_name, c.redirect_uri, c.code_challenge, c.scopes, c.resource;

-- name: CreateOAuthGrant :exec
INSERT INTO oauth_grants (id, user_id, client_id, client_name, scopes, resource, created_at, last_used_at)
VALUES (@id, @user_id, @client_id, @client_name, @scopes, @resource, @now, NULL);

-- name: CreateOAuthToken :exec
INSERT INTO oauth_tokens (token_hash, grant_id, kind, created_at, expires_at, used_at)
VALUES (@token_hash, @grant_id, @kind, @now, @expires_at, NULL);

-- name: GetOAuthAccessToken :one
-- アクセストークンから、連携と、そのユーザーを引く。期限を過ぎたものと、無効にしたユーザーのものは返さない。
SELECT g.id AS grant_id, g.client_id, g.client_name, g.scopes, g.resource, g.last_used_at, u.id AS user_id, u.role
FROM oauth_tokens t
JOIN oauth_grants g ON g.id = t.grant_id
JOIN users u ON u.id = g.user_id
WHERE t.token_hash = @token_hash AND t.kind = 'access' AND t.expires_at > @now AND u.disabled_at IS NULL;

-- name: LockOAuthRefreshToken :one
-- リフレッシュトークンと、その連携を、入れ替えが重ならないようロックして引く。
SELECT t.used_at, t.expires_at, g.id AS grant_id, g.user_id, g.client_id, g.scopes, g.resource, u.role, u.disabled_at AS user_disabled_at
FROM oauth_tokens t
JOIN oauth_grants g ON g.id = t.grant_id
JOIN users u ON u.id = g.user_id
WHERE t.token_hash = @token_hash AND t.kind = 'refresh'
FOR UPDATE OF t;

-- name: MarkOAuthRefreshTokenUsed :exec
UPDATE oauth_tokens SET used_at = @now WHERE token_hash = @token_hash;

-- name: TouchOAuthGrant :exec
UPDATE oauth_grants SET last_used_at = @now WHERE id = @id;

-- name: DeleteOAuthGrant :exec
DELETE FROM oauth_grants WHERE id = @id;

-- name: GetOAuthTokenForRevocation :one
-- 失効させるトークンの種類と連携を引く。ほかのクライアントのトークンは返さない。
SELECT t.kind, t.grant_id
FROM oauth_tokens t
JOIN oauth_grants g ON g.id = t.grant_id
WHERE t.token_hash = @token_hash AND g.client_id = @client_id;

-- name: DeleteOAuthToken :exec
DELETE FROM oauth_tokens WHERE token_hash = @token_hash;

-- name: DeleteExpiredOAuthAuthorizationCodes :execrows
DELETE FROM oauth_authorization_codes WHERE code_hash IN (
  SELECT c.code_hash FROM oauth_authorization_codes c WHERE c.expires_at <= @now LIMIT @max_count
);

-- name: DeleteExpiredOAuthTokens :execrows
DELETE FROM oauth_tokens WHERE token_hash IN (
  SELECT t.token_hash FROM oauth_tokens t WHERE t.expires_at <= @now LIMIT @max_count
);

-- name: DeleteOAuthGrantsWithoutTokens :execrows
-- トークンがすべて期限を過ぎて消えた連携を消す。リフレッシュトークンを 30 日使わなかった連携は、ここで終わる。
DELETE FROM oauth_grants
WHERE id IN (
  SELECT g.id FROM oauth_grants g
  WHERE g.created_at <= @created_before AND NOT EXISTS (SELECT 1 FROM oauth_tokens t WHERE t.grant_id = g.id)
  LIMIT @max_count
);
