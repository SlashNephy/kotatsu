-- name: ListReservationsToFollow :many
-- 番組を指定した予約のうち、予約か番組の終わりがまだ来ていないものと、その番組の今の EPG。
-- 予約の終わりが過ぎた後は、同じ放送の終わりが延びたとき (番組が予約の終わりより前に始まる) だけ追従する。
-- 番組が予約の終わりより後に始まるなら、EID が別の番組に再利用されたものとみなし、古い予約を書き換えない (#510)。
-- ルール予約の録画設定は、一致したルールのうち最も古いものの上書きを使う。
SELECT r.id, r.kind, r.enabled, r.service_id, r.event_id, r.start_at, r.end_at, r.name, r.description, r.extended, r.genres,
  r.follow_event_relay, r.settings,
  (
    SELECT ru.settings FROM reservation_rules rr JOIN rules ru ON ru.id = rr.rule_id
    WHERE rr.reservation_id = r.id
    ORDER BY ru.created_at, ru.id
    LIMIT 1
  )::jsonb AS rule_settings,
  p.start_at AS program_start_at, p.end_at AS program_end_at, p.end_undecided AS program_end_undecided,
  p.name AS program_name, p.description AS program_description, p.extended AS program_extended,
  p.genres AS program_genres, p.related_items AS program_related_items
FROM reservations r
JOIN programs p ON p.service_id = r.service_id AND p.event_id = r.event_id
WHERE r.event_id IS NOT NULL
  AND (
    coalesce(r.planned_end_at, r.end_at) > @now::timestamptz
    OR (p.end_at > @now::timestamptz AND p.start_at < coalesce(r.planned_end_at, r.end_at))
  )
ORDER BY r.id;

-- name: UpdateReservationProgram :exec
UPDATE reservations SET
  start_at = @start_at,
  end_at = @end_at,
  name = @name,
  description = @description,
  extended = @extended,
  genres = @genres,
  updated_at = @now
WHERE id = @id;

-- name: InsertRelayedReservation :one
-- イベントリレーの引き継ぎ先の番組予約を、番組情報をコピーして作る。引き継ぎ先の予約がすでにあれば何もせず、行を返さない。
-- 番組の終了時刻が未定なら、予約の終わりを、始まりから undecided_seconds 秒後にしておく。決まったら追従で直す (#521)。
INSERT INTO reservations (
  id, kind, service_id, event_id, start_at, end_at, name, description, extended, genres,
  enabled, follow_event_relay, settings, created_at, created_by, updated_at, updated_by, relayed_from
)
SELECT @id, 'program', p.service_id, p.event_id, p.start_at,
  CASE WHEN p.end_undecided OR p.end_at <= p.start_at
    THEN p.start_at + make_interval(secs => @undecided_seconds::double precision)
    ELSE p.end_at END,
  p.name, p.description, p.extended, p.genres,
  true, true, @settings, @now, NULL, @now, NULL, @relayed_from
FROM programs p
WHERE p.service_id = @service_id AND p.event_id = @event_id
ON CONFLICT (service_id, event_id) WHERE event_id IS NOT NULL DO NOTHING
RETURNING id;

-- name: ListFinishedReservations :many
-- 放送が終わって before より前に録画の予定の終わりが過ぎ、録画も終わった予約。
-- 割り振りが終わっていないか、録画中の録画があれば、引き継ぎや延長の途中なので含めない。
-- 割り当ての後に放送が延びると、予定の終わりより番組の終わりが後になるので、遅いほうで比べる。
SELECT r.id
FROM reservations r
WHERE greatest(r.planned_end_at, r.end_at) <= @before::timestamptz
  AND NOT EXISTS (SELECT 1 FROM recording_assignments a WHERE a.reservation_id = r.id AND a.state <> 'finished')
  AND NOT EXISTS (SELECT 1 FROM recordings rec WHERE rec.reservation_id = r.id AND rec.state = 'recording')
ORDER BY r.id;

-- name: ListReservationsToRelay :many
-- イベントリレーで別のサービスに引き継ぐ番組の、有効で追従する予約と、その番組の関連。
-- 延長で臨時のサブチャンネルへ移るときは、移る先の番組が、移った時点で初めて EPG に出ることがあるので、
-- 予約の終わりが過ぎても、since までに終わった予約は追う (#639)。EID が再利用された番組 (予約の終わりより後に始まる) は除く。
SELECT r.id, r.kind, r.service_id, r.settings,
  (
    SELECT ru.settings FROM reservation_rules rr JOIN rules ru ON ru.id = rr.rule_id
    WHERE rr.reservation_id = r.id
    ORDER BY ru.created_at, ru.id
    LIMIT 1
  )::jsonb AS rule_settings,
  p.related_items AS program_related_items
FROM reservations r
JOIN programs p ON p.service_id = r.service_id AND p.event_id = r.event_id
WHERE r.event_id IS NOT NULL AND r.enabled AND r.follow_event_relay
  AND coalesce(r.planned_end_at, r.end_at) > @since::timestamptz
  AND p.start_at < coalesce(r.planned_end_at, r.end_at)
  AND p.related_items @> '[{"type": "relay"}]'::jsonb
ORDER BY r.id;
