-- name: InsertWatchReservation :execrows
-- 視聴予約を付ける。放送が始まった番組と、EPG にない番組には付けない (設計書 4.8)。
-- 視聴ルールや見たいリストの通知 (#1234) が作った視聴予約に手で付け直したら、手で付けたものにする。一致しなくなっても消さない。
INSERT INTO watch_reservations (user_id, service_id, event_id, created_at)
SELECT @user_id, p.service_id, p.event_id, @now
FROM programs p
WHERE p.service_id = @service_id AND p.event_id = @event_id AND p.start_at > @now
ON CONFLICT (user_id, service_id, event_id) DO UPDATE SET watch_rule_id = NULL, from_wishlist = false;

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
-- 通知した番組は、手で外した番組と同じく覚え、視聴ルールが付け直さないようにする。放送が遅れたり、通知を何分前に送るかを
-- 縮めたりして、通知の時刻がまた先になっても、二度は通知しない (#1093)。
WITH due AS (
  DELETE FROM watch_reservations w
  USING programs p, users u
  LEFT JOIN user_settings us ON us.user_id = u.id
  WHERE p.service_id = w.service_id AND p.event_id = w.event_id
    AND u.id = w.user_id
    AND p.start_at - make_interval(mins => coalesce(us.watch_reminder_minutes, 5)) <= @now
    AND p.end_at > @now
  RETURNING w.user_id, w.service_id, w.event_id
), notified AS (
  INSERT INTO watch_rule_dismissals (user_id, service_id, event_id, created_at)
  SELECT due.user_id, due.service_id, due.event_id, @now FROM due
  ON CONFLICT (user_id, service_id, event_id) DO NOTHING
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

-- name: ListWishlistWatchUsers :many
-- 見たいリストの作品の放送を通知する (#1234) ユーザーと、見たいリストの通知が付けた視聴予約を持つユーザーを返す。
-- 通知をやめたユーザーの視聴予約も、照合で消すため。
SELECT user_id, notify_wishlist FROM anime_record_settings WHERE notify_wishlist
UNION
SELECT DISTINCT w.user_id, coalesce(s.notify_wishlist, false) AS notify_wishlist
FROM watch_reservations w LEFT JOIN anime_record_settings s ON s.user_id = w.user_id
WHERE w.from_wishlist
ORDER BY user_id;

-- name: ListWishlistWatchCandidates :many
-- ユーザーの見たいリストの作品の、これからの本放送のうち、話ごとに最も早い放送を返す (#1234)。
-- 見終えた話数までの話、再放送、話数のない放送 (特番など)、終了時刻の決まっていない番組は除く。
-- 同じ話を複数の局で放送するときは、最も早い放送 (同じ時刻ならサービス ID の小さい局) だけにし、二度通知しない。
SELECT DISTINCT ON (e.syobocal_tid, e.count_start) p.service_id, p.event_id
FROM anime_wishlist_entries w
JOIN anime_works aw ON ((w.destination = 'annict' AND aw.annict_work_id = w.work_id) OR (w.destination = 'anilist' AND aw.anilist_id = w.work_id))
  -- TID ごとに、自前の対応 (annict) があればそれを、なければ arm の対応を使う (#1223)。
  AND (aw.source = 'annict' OR NOT EXISTS (SELECT 1 FROM anime_works o WHERE o.syobocal_tid = aw.syobocal_tid AND o.source = 'annict'))
JOIN program_anime_episodes e ON e.syobocal_tid = aw.syobocal_tid AND NOT e.is_rerun AND e.count_start IS NOT NULL
JOIN program_anime_matches m ON m.service_id = e.service_id AND m.start_at = e.start_at
JOIN programs p ON p.service_id = m.service_id AND p.event_id = m.event_id AND p.start_at = m.start_at
WHERE w.user_id = @user_id AND p.start_at > @now AND NOT p.end_undecided
  AND (w.progress IS NULL OR e.count_end > w.progress)
ORDER BY e.syobocal_tid, e.count_start, p.start_at, p.service_id;

-- name: InsertWishlistWatchReservations :execrows
-- 見たいリストの作品の放送 (service_ids[i], event_ids[i]) に、見たいリストの通知 (#1234) が付けたものとして、ユーザーの視聴予約を付ける。
-- 視聴ルールと同じく、付けてある番組と、手で外した番組と、通知を送る時刻を過ぎた番組には付けない。
INSERT INTO watch_reservations (user_id, service_id, event_id, created_at, watch_rule_id, from_wishlist)
SELECT @user_id, p.service_id, p.event_id, @now, NULL, true
FROM (SELECT unnest(@service_ids::bigint[]) AS service_id, unnest(@event_ids::bigint[]) AS event_id) AS m
JOIN programs p ON p.service_id = m.service_id AND p.event_id = m.event_id
LEFT JOIN user_settings us ON us.user_id = @user_id
WHERE p.start_at - make_interval(mins => coalesce(us.watch_reminder_minutes, 5)) > @now
  AND NOT EXISTS (
    SELECT 1 FROM watch_rule_dismissals d
    WHERE d.user_id = @user_id AND d.service_id = p.service_id AND d.event_id = p.event_id
  )
ON CONFLICT (user_id, service_id, event_id) DO NOTHING;

-- name: DeleteStaleWishlistWatchReservations :execrows
-- 見たいリストの通知 (#1234) が付けた、放送の前の視聴予約のうち、(service_ids[i], event_ids[i]) の組にないものを消す。
-- 作品が見たいリストから外れた、見終えた、通知をやめたときに消すため。手で付けたものと、視聴ルールが付けたものは消さない。
DELETE FROM watch_reservations w
USING programs p
WHERE w.user_id = @user_id AND w.from_wishlist AND p.service_id = w.service_id AND p.event_id = w.event_id AND p.start_at > @now
  AND (w.service_id, w.event_id) NOT IN (SELECT unnest(@service_ids::bigint[]), unnest(@event_ids::bigint[]));
