-- name: InsertWatchReservation :execrows
-- 視聴予約を付ける。放送が始まった番組と、EPG にない番組には付けない (設計書 4.8)。
-- 視聴ルールが作った視聴予約に手で付け直したら、手で付けたものにする。視聴ルールに一致しなくなっても消さない。
INSERT INTO watch_reservations (user_id, service_id, event_id, created_at)
SELECT @user_id, p.service_id, p.event_id, @now
FROM programs p
WHERE p.service_id = @service_id AND p.event_id = @event_id AND p.start_at > @now
ON CONFLICT (user_id, service_id, event_id) DO UPDATE SET watch_rule_id = NULL;

-- name: DeleteWatchReservation :execrows
DELETE FROM watch_reservations WHERE user_id = @user_id AND service_id = @service_id AND event_id = @event_id;

-- name: ListWatchReservedPrograms :many
-- user_id のユーザーが視聴予約した番組のうち、(service_ids[i], event_ids[i]) の組にあるもの。
SELECT service_id, event_id
FROM watch_reservations
WHERE user_id = @user_id
  AND (service_id, event_id) IN (SELECT unnest(@service_ids::bigint[]), unnest(@event_ids::bigint[]));

-- name: ListWatchReservations :many
-- user_id のユーザーの視聴予約を、番組の放送の早い順に返す。EPG にない番組の視聴予約は返さない。
SELECT w.created_at, p.service_id, p.event_id, p.start_at, p.end_at, p.end_undecided, p.is_free, p.name, p.description,
  p.extended, p.genres, p.audios, p.related_items
FROM watch_reservations w
JOIN programs p ON p.service_id = w.service_id AND p.event_id = w.event_id
WHERE w.user_id = @user_id
ORDER BY p.start_at, p.service_id, p.event_id;

-- name: TakeDueWatchReservations :many
-- 通知を送る時刻 (放送の開始からユーザーの設定の分だけ前) を過ぎ、放送が終わっていない視聴予約を消して返す。
-- 消した行を返すので、nekonote が何台あっても、同じ視聴予約を二度は通知しない。
WITH due AS (
  DELETE FROM watch_reservations w
  USING programs p, users u
  LEFT JOIN user_settings us ON us.user_id = u.id
  WHERE p.service_id = w.service_id AND p.event_id = w.event_id
    AND u.id = w.user_id
    AND p.start_at - make_interval(mins => coalesce(us.watch_reminder_minutes, 5)) <= @now
    AND p.end_at > @now
  RETURNING w.user_id, w.service_id, w.event_id
)
SELECT due.user_id, due.service_id, due.event_id, p.name, p.start_at, s.name AS service_name
FROM due
JOIN programs p ON p.service_id = due.service_id AND p.event_id = due.event_id
JOIN services s ON s.id = due.service_id
ORDER BY p.start_at, due.service_id, due.event_id, due.user_id;

-- name: PruneWatchReservations :execrows
-- 放送が終わった番組と、EPG から消えた番組の視聴予約を消す。
DELETE FROM watch_reservations w
WHERE NOT EXISTS (
  SELECT 1 FROM programs p WHERE p.service_id = w.service_id AND p.event_id = w.event_id AND p.end_at > @now
);

-- name: GetWatchReminderMinutes :one
-- 視聴予約の通知を、放送の何分前に送るか。一度も変えていなければ、既定値を返す。
SELECT coalesce(s.watch_reminder_minutes, 5)::integer AS watch_reminder_minutes
FROM (SELECT @user_id::uuid AS id) AS u
LEFT JOIN user_settings s ON s.user_id = u.id;

-- name: UpsertWatchReminderMinutes :one
INSERT INTO user_settings (user_id, watch_reminder_minutes, updated_at)
VALUES (@user_id, @watch_reminder_minutes, @now)
ON CONFLICT (user_id) DO UPDATE SET
  watch_reminder_minutes = EXCLUDED.watch_reminder_minutes,
  updated_at = EXCLUDED.updated_at
RETURNING watch_reminder_minutes;
