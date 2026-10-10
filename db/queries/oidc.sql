-- name: InsertOIDCLogin :exec
INSERT INTO oidc_logins (id, token_hash, state, nonce, code_verifier, redirect_uri, created_at, expires_at)
VALUES (@id, @token_hash, @state, @nonce, @code_verifier, @redirect_uri, @now, @expires_at);

-- name: TakeOIDCLogin :one
-- ログインの途中の状態は 1 回しか使わせないので、引くと同時に消す。
DELETE FROM oidc_logins WHERE token_hash = @token_hash AND expires_at > @now
RETURNING *;

-- name: DeleteExpiredOIDCLogins :execrows
DELETE FROM oidc_logins WHERE expires_at <= @now;

-- name: GetOIDCIdentity :one
SELECT * FROM user_oidc_identities WHERE issuer = @issuer AND subject = @subject;

-- name: InsertOIDCIdentity :exec
INSERT INTO user_oidc_identities (id, user_id, issuer, subject, created_at, last_used_at)
VALUES (@id, @user_id, @issuer, @subject, @now, sqlc.narg(last_used_at));

-- name: RecordOIDCIdentityUse :exec
UPDATE user_oidc_identities SET last_used_at = @now WHERE id = @id;

-- name: ListUserOIDCIdentities :many
SELECT * FROM user_oidc_identities WHERE user_id = @user_id ORDER BY created_at, id;
