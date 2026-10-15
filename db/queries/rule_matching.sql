-- name: ListMatchablePrograms :many
-- ルールの対象になる番組。放送が始まった番組と、終了時刻が未定の番組は対象にしない (設計書 4.3)。
-- 映像か音声を流すサービス (media_service_types) の番組だけを対象にする。
-- 照合には正規化した列を使う。詳細の正規化した列がない (列を足す前に書いた) 番組だけ、上流の詳細を返す (#530)。
SELECT p.service_id, p.event_id, p.start_at, p.end_at, p.is_free, p.name, p.name_normalized, p.description_normalized,
  p.extended_normalized, (CASE WHEN p.extended_normalized IS NULL THEN p.extended ELSE '{}' END)::jsonb AS extended,
  p.genres, p.related_items, s.channel_type
FROM programs p
JOIN services s ON s.id = p.service_id
WHERE p.start_at > @now AND NOT p.end_undecided AND s.service_type = ANY(@media_service_types::bigint[])
ORDER BY p.service_id, p.event_id;

-- name: ListLatestRecordingHistory :many
-- 番組名のキーごと、サービスごとの、最後に録画した番組の終了時刻。
SELECT title_key, service_id, max(end_at)::timestamptz AS last_end_at
FROM recording_history
WHERE title_key = ANY(@title_keys::text[])
GROUP BY title_key, service_id;

-- name: ListFutureProgramReservations :many
-- 放送が始まっていない、番組を指定した予約 (番組予約とルール予約)。照合の対象の番組 (ListMatchablePrograms) とそろえる。
-- 録画は開始マージンとチューニングの時間だけ前に始まるので、録画中かどうか (recording) も返す。録画中の予約は、照合で消さない (設計書 4.3)。
-- 番組が EPG にあり、終了時刻が決まっているか (decided) も返す。番組が消えたか、未定になったのは一時的なことがあるので、
-- ルールに一致しなくなったとはみなさない (#518)。
SELECT r.id, r.kind, r.service_id, r.event_id, r.enabled, r.follow_event_relay,
  EXISTS (SELECT 1 FROM recordings rec WHERE rec.reservation_id = r.id AND rec.state = 'recording') AS recording,
  EXISTS (SELECT 1 FROM programs p WHERE p.service_id = r.service_id AND p.event_id = r.event_id AND NOT p.end_undecided) AS decided
FROM reservations r
WHERE r.event_id IS NOT NULL AND r.start_at > @now
ORDER BY r.id;

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

-- name: SetReservationFollowEventRelay :exec
UPDATE reservations SET follow_event_relay = @follow_event_relay, updated_at = @now WHERE id = @id;
