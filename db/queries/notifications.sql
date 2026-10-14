-- name: InsertNotificationEvent :exec
INSERT INTO notification_events (id, kind, occurred_at, payload, dispatched_at, audience, by_rule)
VALUES (@id, @kind, @occurred_at, @payload, NULL, sqlc.narg(audience)::uuid[], @by_rule);

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
-- audience が NULL でなければ、その中のユーザーだけにする。by_rule なら、ルール予約の録画の通知を受け取らないユーザーを除く。
SELECT users.id FROM users
LEFT JOIN notification_preferences p ON p.user_id = users.id AND p.kind = @kind
WHERE users.disabled_at IS NULL AND COALESCE(p.in_app, true)
  AND (sqlc.narg(audience)::uuid[] IS NULL OR users.id = ANY(sqlc.narg(audience)::uuid[]))
  AND (NOT @by_rule::boolean OR COALESCE(p.rule_reservations, true))
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
INSERT INTO notification_preferences (user_id, kind, in_app, web_push, rule_reservations, updated_at)
VALUES (@user_id, @kind, @in_app, @web_push, @rule_reservations, @now)
ON CONFLICT (user_id, kind) DO UPDATE SET in_app = EXCLUDED.in_app, web_push = EXCLUDED.web_push,
  rule_reservations = EXCLUDED.rule_reservations, updated_at = EXCLUDED.updated_at;

-- name: GetReservationForNotification :one
-- 競合の通知に載せる、予約の番組名とサービス名。
SELECT reservations.id, reservations.name, reservations.start_at, reservations.end_at, services.name AS service_name
FROM reservations JOIN services ON services.id = reservations.service_id
WHERE reservations.id = @id;

-- name: GetNotificationEvent :one
SELECT * FROM notification_events WHERE id = @id;

-- name: GetReservationAudience :one
-- 予約の通知を受け取るユーザーを決める材料。手動の予約なら予約したユーザー、ルール予約なら一致したルールを作ったユーザー。
-- rule_ids と rule_names は、ルールを作った順に並べる。rule_creators は順不同で、ルールを作ったユーザーが消えていれば入れない。
SELECT r.kind, r.created_by,
  COALESCE(array_agg(ru.id ORDER BY ru.created_at, ru.id) FILTER (WHERE ru.id IS NOT NULL), '{}')::uuid[] AS rule_ids,
  COALESCE(array_agg(ru.name ORDER BY ru.created_at, ru.id) FILTER (WHERE ru.id IS NOT NULL), '{}')::text[] AS rule_names,
  COALESCE(array_agg(DISTINCT ru.created_by) FILTER (WHERE ru.created_by IS NOT NULL), '{}')::uuid[] AS rule_creators
FROM reservations r
LEFT JOIN reservation_rules rr ON rr.reservation_id = r.id
LEFT JOIN rules ru ON ru.id = rr.rule_id
WHERE r.id = @id
GROUP BY r.id;
