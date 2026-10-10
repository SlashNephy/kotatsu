-- name: InsertNotificationEvent :exec
INSERT INTO notification_events (id, kind, occurred_at, payload, dispatched_at)
VALUES (@id, @kind, @occurred_at, @payload, NULL);

-- name: ListPendingNotificationEvents :many
-- 配っていないイベントを、古い順に引き、ロックする。ほかの nekonote が同じイベントを配らないよう、ロック中の行は飛ばす。
SELECT * FROM notification_events WHERE dispatched_at IS NULL
ORDER BY occurred_at, id
LIMIT @max_count
FOR UPDATE SKIP LOCKED;

-- name: MarkNotificationEventDispatched :exec
UPDATE notification_events SET dispatched_at = @now WHERE id = @id;

-- name: ListNotificationRecipients :many
-- イベントの種類 kind を画面内の通知で受け取る、有効なユーザーを返す。設定がなければ受け取る。
SELECT users.id FROM users
LEFT JOIN notification_preferences p ON p.user_id = users.id AND p.kind = @kind
WHERE users.disabled_at IS NULL AND COALESCE(p.in_app, true)
ORDER BY users.id;

-- name: InsertNotifications :exec
-- イベント event_id を、ユーザー user_ids に配る。ids と user_ids は同じ順に並べる。
INSERT INTO notifications (id, user_id, event_id, created_at, read_at)
SELECT unnest(@ids::uuid[]), unnest(@user_ids::uuid[]), @event_id, @now, NULL
ON CONFLICT (user_id, event_id) DO NOTHING;

-- name: ListNotifications :many
-- ユーザーの通知を、新しい順に返す。before_at と before_id より前 (古い) の通知だけを返し、ページを送る。
-- before_at と before_id は、両方を渡すか、両方を NULL にする。
SELECT sqlc.embed(notifications), sqlc.embed(notification_events)
FROM notifications
JOIN notification_events ON notification_events.id = notifications.event_id
WHERE notifications.user_id = @user_id
  AND (NOT @unread_only::boolean OR notifications.read_at IS NULL)
  AND (sqlc.narg(before_at)::timestamptz IS NULL
    OR (notifications.created_at, notifications.id) < (sqlc.narg(before_at)::timestamptz, sqlc.narg(before_id)::uuid))
ORDER BY notifications.created_at DESC, notifications.id DESC
LIMIT @max_count;

-- name: CountUnreadNotifications :one
SELECT count(*) FROM notifications WHERE user_id = @user_id AND read_at IS NULL;

-- name: MarkNotificationsRead :execrows
UPDATE notifications SET read_at = @now WHERE user_id = @user_id AND id = ANY(@ids::uuid[]) AND read_at IS NULL;

-- name: MarkAllNotificationsRead :execrows
UPDATE notifications SET read_at = @now WHERE user_id = @user_id AND read_at IS NULL;

-- name: ListNotificationPreferences :many
SELECT * FROM notification_preferences WHERE user_id = @user_id ORDER BY kind;

-- name: UpsertNotificationPreference :exec
INSERT INTO notification_preferences (user_id, kind, in_app, web_push, updated_at)
VALUES (@user_id, @kind, @in_app, @web_push, @now)
ON CONFLICT (user_id, kind) DO UPDATE SET in_app = EXCLUDED.in_app, web_push = EXCLUDED.web_push, updated_at = EXCLUDED.updated_at;

-- name: GetReservationForNotification :one
-- 競合の通知に載せる、予約の番組名とサービス名。
SELECT reservations.id, reservations.name, reservations.start_at, reservations.end_at, services.name AS service_name
FROM reservations JOIN services ON services.id = reservations.service_id
WHERE reservations.id = @id;
