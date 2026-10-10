-- name: ListUserPasskeys :many
SELECT * FROM user_passkeys WHERE user_id = @user_id ORDER BY created_at, id;

-- name: CountUserPasskeys :one
SELECT count(*) FROM user_passkeys WHERE user_id = @user_id;

-- name: GetPasskeyByCredentialID :one
SELECT * FROM user_passkeys WHERE credential_id = @credential_id;

-- name: InsertUserPasskey :exec
INSERT INTO user_passkeys (id, user_id, credential_id, credential, rp_id, name, created_at, last_used_at)
VALUES (@id, @user_id, @credential_id, @credential, @rp_id, @name, @now, NULL);

-- name: RecordPasskeyUse :exec
UPDATE user_passkeys SET credential = @credential, last_used_at = @now WHERE id = @id;

-- name: RenameUserPasskey :execrows
UPDATE user_passkeys SET name = @name WHERE id = @id AND user_id = @user_id;

-- name: DeleteUserPasskey :execrows
DELETE FROM user_passkeys WHERE id = @id AND user_id = @user_id;

-- name: InsertWebAuthnCeremony :exec
INSERT INTO webauthn_ceremonies (id, token_hash, kind, user_id, session, created_at, expires_at)
VALUES (@id, @token_hash, @kind, @user_id, @session, @now, @expires_at);

-- name: TakeWebAuthnCeremony :one
-- 手続きは 1 回しか使わせないので、引くと同時に消す。
DELETE FROM webauthn_ceremonies WHERE token_hash = @token_hash AND kind = @kind AND expires_at > @now
RETURNING *;

-- name: DeleteExpiredWebAuthnCeremonies :execrows
DELETE FROM webauthn_ceremonies WHERE expires_at <= @now;

-- name: SetLoginChallengeWebAuthnSession :exec
UPDATE login_challenges SET webauthn_session = @webauthn_session WHERE id = @id;
