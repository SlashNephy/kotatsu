-- name: UpsertWebPushSubscription :exec
-- 購読を保存する。同じ端末 (endpoint) を別のユーザーが購読し直したら、そのユーザーの購読に移す。
INSERT INTO web_push_subscriptions (id, user_id, endpoint, p256dh, auth, user_agent, created_at, updated_at)
VALUES (@id, @user_id, @endpoint, @p256dh, @auth, @user_agent, @now, @now)
ON CONFLICT (endpoint) DO UPDATE SET
  user_id = EXCLUDED.user_id, p256dh = EXCLUDED.p256dh, auth = EXCLUDED.auth,
  user_agent = EXCLUDED.user_agent, updated_at = EXCLUDED.updated_at;

-- name: DeleteWebPushSubscription :execrows
DELETE FROM web_push_subscriptions WHERE user_id = @user_id AND endpoint = @endpoint;

-- name: DeleteExpiredWebPushSubscription :exec
-- プッシュサービスが失効を返した購読を消す。送った後に、同じ端末が購読し直していたら (鍵が変わっていたら)、消さない。
DELETE FROM web_push_subscriptions WHERE id = @id AND p256dh = @p256dh AND auth = @auth;

-- name: GetWebPushSubscription :one
SELECT * FROM web_push_subscriptions WHERE id = @id;

-- name: ListWebPushSubscriptions :many
SELECT * FROM web_push_subscriptions WHERE user_id = @user_id ORDER BY created_at, id;

-- name: ListWebPushSubscriptionsForKind :many
-- イベントの種類 kind を Web Push で受け取る、有効なユーザーの購読の ID を返す。設定がなければ受け取る。
-- audience と by_rule の扱いは、ListNotificationRecipients と同じ。
SELECT s.id FROM web_push_subscriptions s
JOIN users ON users.id = s.user_id
LEFT JOIN notification_preferences p ON p.user_id = s.user_id AND p.kind = @kind
WHERE users.disabled_at IS NULL AND COALESCE(p.web_push, true)
  AND (sqlc.narg(audience)::uuid[] IS NULL OR users.id = ANY(sqlc.narg(audience)::uuid[]))
  AND (NOT @by_rule::boolean OR COALESCE(p.rule_reservations, true))
ORDER BY s.id;

-- name: InsertVAPIDKeys :exec
-- VAPID の鍵を保存する。ほかのインスタンスが先に作っていたら、そちらを使うので、何もしない。
INSERT INTO vapid_keys (id, public_key, private_key_ciphertext, created_at)
VALUES (true, @public_key, @private_key_ciphertext, @now)
ON CONFLICT (id) DO NOTHING;

-- name: GetVAPIDKeys :one
SELECT * FROM vapid_keys WHERE id;
