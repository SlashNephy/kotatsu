-- name: BumpUserSettingsVersion :one
INSERT INTO user_setting_versions (user_id, version, updated_at)
VALUES (@user_id, 1, @now)
ON CONFLICT (user_id) DO UPDATE SET version = user_setting_versions.version + 1, updated_at = EXCLUDED.updated_at
RETURNING version;

-- name: GetUserSettingsVersion :one
SELECT coalesce((SELECT version FROM user_setting_versions WHERE user_id = @user_id), 0)::bigint AS version;

-- name: GetPlayerSettings :one
-- プレイヤーの設定。一度も変えていなければ、既定値を返す。
SELECT
  coalesce(s.jikkyo_enabled, true)::boolean AS jikkyo_enabled,
  coalesce(s.captions_enabled, true)::boolean AS captions_enabled
FROM (SELECT @user_id::uuid AS id) AS u
LEFT JOIN user_settings s ON s.user_id = u.id;

-- name: UpsertPlayerSettings :one
-- プレイヤーの設定を変える。null の項目は変えない。
INSERT INTO user_settings (user_id, jikkyo_enabled, captions_enabled, updated_at)
VALUES (@user_id, coalesce(sqlc.narg(jikkyo_enabled)::boolean, true), coalesce(sqlc.narg(captions_enabled)::boolean, true), @now)
ON CONFLICT (user_id) DO UPDATE SET
  jikkyo_enabled = coalesce(sqlc.narg(jikkyo_enabled)::boolean, user_settings.jikkyo_enabled),
  captions_enabled = coalesce(sqlc.narg(captions_enabled)::boolean, user_settings.captions_enabled),
  updated_at = EXCLUDED.updated_at
RETURNING jikkyo_enabled, captions_enabled;

-- name: ListPinnedServiceIDs :many
-- ユーザーがピン留めしたサービスを、ピン留めした順に返す (#485)。
SELECT service_id FROM pinned_services WHERE user_id = @user_id ORDER BY created_at, service_id;

-- name: PinService :exec
INSERT INTO pinned_services (user_id, service_id, created_at) VALUES (@user_id, @service_id, @now)
ON CONFLICT DO NOTHING;

-- name: UnpinService :exec
DELETE FROM pinned_services WHERE user_id = @user_id AND service_id = @service_id;
