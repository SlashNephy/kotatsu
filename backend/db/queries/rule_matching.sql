-- name: ListMatchablePrograms :many
-- ルールの対象になる番組。放送が始まった番組と、終了時刻が未定の番組は対象にしない (設計書 4.3)。
-- 映像か音声を流すサービス (media_service_types) の番組だけを対象にする。
-- 照合には正規化した列を使う。詳細の正規化した列がない (列を足す前に書いた) 番組だけ、上流の詳細を返す (#530)。
SELECT p.service_id, p.event_id, p.start_at, p.end_at, p.is_free, p.name, p.name_normalized, p.description_normalized,
  p.extended_normalized, (CASE WHEN p.extended_normalized IS NULL THEN p.extended ELSE '{}' END)::jsonb AS extended,
  p.genres, p.related_items, p.series_episode, s.channel_type, s.channel
FROM programs p
JOIN services s ON s.id = p.service_id
WHERE p.start_at > @now AND NOT p.end_undecided AND s.service_type = ANY(@media_service_types::bigint[])
ORDER BY p.service_id, p.event_id;

-- name: ListCancelledBroadcasts :many
-- 人が取り消した (無効にした) 番組の予約のうち、まだ終わっていない、番組名のあるものの放送 (#1504、設計書 4.3)。
-- 照合で、同じ放送が別のイベント ID で送り直されても、予約し直さないために使う。
-- 録り始めに同じ回と分かって Kotatsu が無効にした予約 (#1500) も、同じ放送をもう一度予約しないよう、人が取り消したものと同じに扱う。
SELECT r.service_id, r.event_id, r.name, r.start_at, s.channel_type, s.channel
FROM reservations r
JOIN services s ON s.id = r.service_id
WHERE NOT r.enabled AND r.event_id IS NOT NULL AND r.end_at > @now AND r.name <> ''
ORDER BY r.service_id, r.event_id;

-- name: ListLatestRecordingHistory :many
-- 番組名のキーごと、サービスごとの、最後に録画した番組の終了時刻。
SELECT title_key, service_id, max(end_at)::timestamptz AS last_end_at
FROM recording_history
WHERE title_key = ANY(@title_keys::text[])
GROUP BY title_key, service_id;

-- name: ListProgramAnimeEpisodesForBundling :many
-- 局をまたいで同じ回を束ねる (#1498) 番組のうち、1 つの話に結びついたものの、しょぼいカレンダーの TID と話数 (設計書 3.8)。
-- 2 話連続の放送のように、複数の話に結びついた番組は返さない。
SELECT e.service_id, e.start_at, min(e.syobocal_tid)::integer AS syobocal_tid, min(e.count_start)::double precision AS count
FROM program_anime_episodes e
WHERE (e.service_id, e.start_at) IN (SELECT unnest(@service_ids::bigint[]), unnest(@start_ats::timestamptz[]))
GROUP BY e.service_id, e.start_at
HAVING count(*) = 1 AND min(e.syobocal_tid) IS NOT NULL AND min(e.count_start) IS NOT NULL AND min(e.count_start) = min(e.count_end);

-- name: ListConflictingReservedPrograms :many
-- 割り当ての結果が競合の、これから放送する番組の予約 (設計書 4.5)。同じ回を束ねるときに、録れない見込みの放送として扱う (#1498)。
SELECT service_id, event_id::bigint AS event_id
FROM reservations
WHERE assignment_state = 'conflict' AND event_id IS NOT NULL AND start_at > @now;

-- name: ListRecordedEpisodes :many
-- 局をまたいで同じ回を束ねる (#1498) ときの「録った回」。録画履歴のうち、@since より後に始まった録画と、
-- ほかの録画ソフトから引き継いだもの (期間で切らない)、それに録画中の録画。
-- 録画履歴は、正常に終了し、ドロップが閾値以下の録画だけを持つ (設計書 4.4)。消した録画も、履歴は残る。
SELECT h.service_id, h.name, h.description, h.series_episode, h.syobocal_tid, h.anime_count, h.end_at
FROM recording_history h
WHERE h.imported OR h.start_at >= @since
UNION ALL
SELECT rec.service_id, rec.name, rec.description, p.series_episode, a.syobocal_tid, a.count_start AS anime_count, rec.end_at
FROM recordings rec
LEFT JOIN programs p ON p.service_id = rec.service_id AND p.event_id = rec.event_id
LEFT JOIN LATERAL (
  SELECT min(e.syobocal_tid)::integer AS syobocal_tid, min(e.count_start)::double precision AS count_start
  FROM recording_anime_episodes e
  WHERE e.recording_id = rec.id
  HAVING count(*) = 1 AND min(e.count_start) = min(e.count_end)
) a ON true
WHERE rec.state = 'recording';

-- name: ListFutureProgramReservations :many
-- 放送が始まっていない、番組を指定した予約 (番組予約とルール予約)。照合の対象の番組 (ListMatchablePrograms) とそろえる。
-- 録画は開始マージンとチューニングの時間だけ前に始まるので、録画中かどうか (recording) も返す。録画中の予約は、照合で消さない (設計書 4.3)。
-- 番組が EPG にあり、終了時刻が決まっているか (decided) も返す。番組が消えたか、未定になったのは一時的なことがあるので、
-- ルールに一致しなくなったとはみなさない (#518)。
-- 番組名が空か (unnamed) も返す。条件から外れたのか、まだ読めていないだけなのかを区別できないので、消さない (#1501)。
SELECT r.id, r.kind, r.service_id, r.event_id, r.start_at, r.enabled, r.follow_event_relay,
  EXISTS (SELECT 1 FROM recordings rec WHERE rec.reservation_id = r.id AND rec.state = 'recording') AS recording,
  EXISTS (SELECT 1 FROM programs p WHERE p.service_id = r.service_id AND p.event_id = r.event_id AND NOT p.end_undecided) AS decided,
  EXISTS (SELECT 1 FROM programs p WHERE p.service_id = r.service_id AND p.event_id = r.event_id AND p.name = '') AS unnamed
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
