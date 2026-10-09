-- name: ListMatchablePrograms :many
-- ルールの対象になる番組。放送が始まった番組と、終了時刻が未定の番組は対象にしない (設計書 4.3)。
SELECT p.service_id, p.event_id, p.start_at, p.end_at, p.is_free, p.name, p.description,
  p.extended, p.genres, p.related_items, s.channel_type
FROM programs p
JOIN services s ON s.id = p.service_id
WHERE p.start_at > @now AND NOT p.end_undecided
ORDER BY p.service_id, p.event_id;

-- name: ListLatestRecordingHistory :many
-- 番組名のキーごと、サービスごとの、最後に録画した番組の終了時刻。
SELECT title_key, service_id, max(end_at)::timestamptz AS last_end_at
FROM recording_history
WHERE title_key = ANY(@title_keys::text[])
GROUP BY title_key, service_id;

-- name: ListFutureProgramReservations :many
-- 放送が始まっていない、番組を指定した予約 (番組予約とルール予約)。
SELECT id, kind, service_id, event_id, enabled
FROM reservations
WHERE event_id IS NOT NULL AND start_at > @now
ORDER BY id;

-- name: ListFutureReservationRules :many
SELECT rr.reservation_id, rr.rule_id
FROM reservation_rules rr
JOIN reservations r ON r.id = rr.reservation_id
WHERE r.event_id IS NOT NULL AND r.start_at > @now;

-- name: InsertReservationRule :exec
INSERT INTO reservation_rules (reservation_id, rule_id) VALUES (@reservation_id, @rule_id)
ON CONFLICT DO NOTHING;

-- name: DeleteReservationRule :exec
DELETE FROM reservation_rules WHERE reservation_id = @reservation_id AND rule_id = @rule_id;
