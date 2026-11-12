-- name: UpsertWebPushSubscription :exec
-- 購読を保存する。同じ端末 (endpoint) を別のユーザーが購読し直したら、そのユーザーの購読に移す。
INSERT INTO web_push_subscriptions (id, user_id, endpoint, p256dh, auth, user_agent, session_id, created_at, updated_at)
VALUES (@id, @user_id, @endpoint, @p256dh, @auth, @user_agent, sqlc.narg(session_id), @now, @now)
ON CONFLICT (endpoint) DO UPDATE SET
  user_id = EXCLUDED.user_id, p256dh = EXCLUDED.p256dh, auth = EXCLUDED.auth,
  user_agent = EXCLUDED.user_agent, session_id = EXCLUDED.session_id, updated_at = EXCLUDED.updated_at;

-- name: DeleteWebPushSubscription :execrows
DELETE FROM web_push_subscriptions WHERE user_id = @user_id AND endpoint = @endpoint;

-- name: DeleteWebPushSubscriptionsOfSession :exec
-- 失効させたセッションで購読した購読を消す (#1365)。
DELETE FROM web_push_subscriptions WHERE session_id = @session_id;

-- name: DeleteWebPushSubscriptionsOfOtherSessions :exec
-- ユーザーの keep_id のほかのセッションで購読した購読を消す (#1365)。
-- keep_id が NULL なら、ユーザーのセッションに結びついた購読をすべて消す。
DELETE FROM web_push_subscriptions
WHERE user_id = @user_id AND session_id IS NOT NULL AND (sqlc.narg(keep_id)::uuid IS NULL OR session_id <> sqlc.narg(keep_id)::uuid);

-- name: DeleteExpiredWebPushSubscription :exec
-- プッシュサービスが失効を返した購読を消す。送った後に、同じ端末が購読し直していたら (鍵が変わっていたら)、消さない。
DELETE FROM web_push_subscriptions WHERE id = @id AND p256dh = @p256dh AND auth = @auth;

-- name: GetWebPushSubscription :one
SELECT * FROM web_push_subscriptions WHERE id = @id;

-- name: ListWebPushSubscriptions :many
SELECT * FROM web_push_subscriptions WHERE user_id = @user_id ORDER BY created_at, id;

-- name: ListWebPushSubscriptionsForKind :many
-- イベントの種類 kind を Web Push で受け取る、有効なユーザーの購読の ID と、その持ち主を返す。設定がなければ受け取る。
-- audience と by_rule の扱いは、ListNotificationRecipients と同じ。
SELECT s.id, s.user_id FROM web_push_subscriptions s
JOIN users ON users.id = s.user_id
LEFT JOIN notification_preferences p ON p.user_id = s.user_id AND p.kind = @kind
WHERE users.disabled_at IS NULL AND COALESCE(p.web_push, true)
  AND (sqlc.narg(audience)::uuid[] IS NULL OR users.id = ANY(sqlc.narg(audience)::uuid[]))
  AND (NOT @by_rule::boolean OR COALESCE(p.rule_reservations, true))
ORDER BY s.id;

-- name: WebPushSubscriptionReceives :one
-- 購読 id が、今もイベントの種類 kind を Web Push で受け取るかを返す。ListWebPushSubscriptionsForKind と同じ条件で確かめる。
-- 配信のジョブを積んでから送るまでの間に、ユーザーを無効にしたり、設定を変えたりしたら、送らないために使う (#1435)。
SELECT EXISTS (
  SELECT 1 FROM web_push_subscriptions s
  JOIN users ON users.id = s.user_id
  LEFT JOIN notification_preferences p ON p.user_id = s.user_id AND p.kind = @kind
  WHERE s.id = @id AND users.disabled_at IS NULL AND COALESCE(p.web_push, true)
    AND (sqlc.narg(audience)::uuid[] IS NULL OR users.id = ANY(sqlc.narg(audience)::uuid[]))
    AND (NOT @by_rule::boolean OR COALESCE(p.rule_reservations, true))
);

-- name: InsertVAPIDKeys :exec
-- VAPID の鍵を保存する。ほかのインスタンスが先に作っていたら、そちらを使うので、何もしない。
INSERT INTO vapid_keys (id, public_key, private_key_ciphertext, created_at)
VALUES (true, @public_key, @private_key_ciphertext, @now)
ON CONFLICT (id) DO NOTHING;

-- name: GetVAPIDKeys :one
SELECT * FROM vapid_keys WHERE id;

-- name: ListWebPushSecrets :many
-- ログの報告から伏せるため、全ユーザーの購読の送り先と秘密を返す (#1521)。
SELECT endpoint, auth FROM web_push_subscriptions ORDER BY id;
