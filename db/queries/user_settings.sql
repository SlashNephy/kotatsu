-- name: BumpUserSettingsVersion :one
INSERT INTO user_setting_versions (user_id, version, updated_at)
VALUES (@user_id, 1, @now)
ON CONFLICT (user_id) DO UPDATE SET version = user_setting_versions.version + 1, updated_at = EXCLUDED.updated_at
RETURNING version;

-- name: GetUserSettingsVersion :one
SELECT coalesce((SELECT version FROM user_setting_versions WHERE user_id = @user_id), 0)::bigint AS version;
