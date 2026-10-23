-- name: GetUserTOTPForUpdate :one
SELECT * FROM user_totps WHERE user_id = @user_id FOR UPDATE;

-- name: HasUserTOTP :one
SELECT EXISTS (SELECT 1 FROM user_totps WHERE user_id = @user_id);

-- name: UpsertUserTOTP :exec
INSERT INTO user_totps (user_id, secret_encrypted, last_used_step, created_at)
VALUES (@user_id, @secret_encrypted, @last_used_step, @now)
ON CONFLICT (user_id) DO UPDATE SET
  secret_encrypted = EXCLUDED.secret_encrypted, last_used_step = EXCLUDED.last_used_step, created_at = EXCLUDED.created_at;

-- name: SetTOTPLastUsedStep :exec
UPDATE user_totps SET last_used_step = @last_used_step WHERE user_id = @user_id;

-- name: DeleteUserTOTP :execrows
DELETE FROM user_totps WHERE user_id = @user_id;

-- name: UpsertTOTPEnrollment :exec
INSERT INTO totp_enrollments (user_id, secret_encrypted, created_at, expires_at)
VALUES (@user_id, @secret_encrypted, @now, @expires_at)
ON CONFLICT (user_id) DO UPDATE SET
  secret_encrypted = EXCLUDED.secret_encrypted, created_at = EXCLUDED.created_at, expires_at = EXCLUDED.expires_at;

-- name: GetTOTPEnrollmentForUpdate :one
SELECT * FROM totp_enrollments WHERE user_id = @user_id AND expires_at > @now FOR UPDATE;

-- name: DeleteTOTPEnrollment :exec
DELETE FROM totp_enrollments WHERE user_id = @user_id;

-- name: DeleteRecoveryCodes :exec
DELETE FROM user_recovery_codes WHERE user_id = @user_id;

-- name: InsertRecoveryCode :exec
INSERT INTO user_recovery_codes (id, user_id, code_hash, created_at, used_at)
VALUES (@id, @user_id, @code_hash, @now, NULL);

-- name: UseRecoveryCode :execrows
UPDATE user_recovery_codes SET used_at = @now
WHERE user_id = @user_id AND code_hash = @code_hash AND used_at IS NULL;

-- name: CountUnusedRecoveryCodes :one
SELECT count(*) FROM user_recovery_codes WHERE user_id = @user_id AND used_at IS NULL;
