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
  coalesce('nx_jikkyo' = ANY(s.comment_sources), false)::boolean AS jikkyo_enabled,
  coalesce(s.comment_sources, '{}')::text[] AS comment_sources,
  coalesce(s.captions_enabled, true)::boolean AS captions_enabled,
  s.comment_settings,
  s.caption_settings
FROM (SELECT @user_id::uuid AS id) AS u
LEFT JOIN user_settings s ON s.user_id = u.id;

-- name: GetCommentSourceAccess :one
-- 実況のコメントのソースを使えるかどうかを決める、全体の設定とユーザーの設定 (#1405、設計書 7.4)。行がなければ空とする。
SELECT
  coalesce((SELECT comment_sources FROM system_settings WHERE id = 1), '{}')::text[] AS system_sources,
  coalesce((SELECT comment_sources FROM user_settings WHERE user_id = @user_id), '{}')::text[] AS user_sources;

-- name: UpsertPlayerSettings :one
-- プレイヤーの設定を変える。null の項目は変えない。jikkyo_enabled は、NX-Jikkyo のソースを足すか外す。comment_sources を渡せば、そちらを使う。
INSERT INTO user_settings (user_id, comment_sources, captions_enabled, updated_at)
VALUES (
  @user_id,
  coalesce(sqlc.narg(comment_sources)::text[], CASE WHEN sqlc.narg(jikkyo_enabled)::boolean THEN ARRAY['nx_jikkyo']::text[] ELSE '{}'::text[] END),
  coalesce(sqlc.narg(captions_enabled)::boolean, true), @now
)
ON CONFLICT (user_id) DO UPDATE SET
  comment_sources = CASE
    WHEN sqlc.narg(comment_sources)::text[] IS NOT NULL THEN sqlc.narg(comment_sources)::text[]
    WHEN sqlc.narg(jikkyo_enabled)::boolean AND NOT 'nx_jikkyo' = ANY(user_settings.comment_sources) THEN array_append(user_settings.comment_sources, 'nx_jikkyo')
    WHEN NOT sqlc.narg(jikkyo_enabled)::boolean THEN array_remove(user_settings.comment_sources, 'nx_jikkyo')
    ELSE user_settings.comment_sources
  END,
  captions_enabled = coalesce(sqlc.narg(captions_enabled)::boolean, user_settings.captions_enabled),
  updated_at = EXCLUDED.updated_at
RETURNING ('nx_jikkyo' = ANY(comment_sources))::boolean AS jikkyo_enabled, comment_sources, captions_enabled, comment_settings, caption_settings;

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
