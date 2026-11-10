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
  coalesce(s.jikkyo_enabled, false)::boolean AS jikkyo_enabled,
  coalesce(s.captions_enabled, true)::boolean AS captions_enabled,
  s.comment_settings,
  s.caption_settings
FROM (SELECT @user_id::uuid AS id) AS u
LEFT JOIN user_settings s ON s.user_id = u.id;

-- name: GetJikkyoAccess :one
-- 実況を使えるかどうかを決める、全体の設定とユーザーの設定 (設計書 7.4)。どちらも、行がなければ無効とする。
SELECT
  coalesce((SELECT jikkyo_enabled FROM system_settings WHERE id = 1), false)::boolean AS system_enabled,
  coalesce((SELECT jikkyo_enabled FROM user_settings WHERE user_id = @user_id), false)::boolean AS user_enabled;

-- name: UpsertPlayerSettings :one
-- プレイヤーの設定を変える。null の項目は変えない。
INSERT INTO user_settings (user_id, jikkyo_enabled, captions_enabled, updated_at)
VALUES (@user_id, coalesce(sqlc.narg(jikkyo_enabled)::boolean, false), coalesce(sqlc.narg(captions_enabled)::boolean, true), @now)
ON CONFLICT (user_id) DO UPDATE SET
  jikkyo_enabled = coalesce(sqlc.narg(jikkyo_enabled)::boolean, user_settings.jikkyo_enabled),
  captions_enabled = coalesce(sqlc.narg(captions_enabled)::boolean, user_settings.captions_enabled),
  updated_at = EXCLUDED.updated_at
RETURNING jikkyo_enabled, captions_enabled, comment_settings, caption_settings;

-- name: SetCommentSettings :exec
-- 実況のコメントの表示とミュートの設定を、まるごと置き換える (#488)。
INSERT INTO user_settings (user_id, comment_settings, updated_at)
VALUES (@user_id, @comment_settings, @now)
ON CONFLICT (user_id) DO UPDATE SET comment_settings = EXCLUDED.comment_settings, updated_at = EXCLUDED.updated_at;

-- name: SetCaptionSettings :exec
-- 字幕の見た目と、文字スーパーを出すかどうかの設定を、まるごと置き換える (#491)。
INSERT INTO user_settings (user_id, caption_settings, updated_at)
VALUES (@user_id, @caption_settings, @now)
ON CONFLICT (user_id) DO UPDATE SET caption_settings = EXCLUDED.caption_settings, updated_at = EXCLUDED.updated_at;

-- name: ListPinnedServiceIDs :many
-- ユーザーがピン留めしたサービスを、ピン留めした順に返す (#485)。
SELECT service_id FROM pinned_services WHERE user_id = @user_id ORDER BY created_at, service_id;

-- name: PinService :exec
INSERT INTO pinned_services (user_id, service_id, created_at) VALUES (@user_id, @service_id, @now)
ON CONFLICT DO NOTHING;

-- name: UnpinService :exec
DELETE FROM pinned_services WHERE user_id = @user_id AND service_id = @service_id;

-- name: GetCommentSettings :one
-- 実況のコメントの設定の JSON。行がないか、設定を変えていなければ NULL。
SELECT comment_settings FROM user_settings WHERE user_id = @user_id;
