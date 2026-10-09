-- name: ListReservationsToFollow :many
-- 番組を指定した予約のうち、録画の予定の終わりがまだ来ていないものと、その番組の今の EPG。
SELECT r.id, r.service_id, r.event_id, r.start_at, r.end_at, r.name, r.description, r.extended, r.genres,
  r.follow_event_relay, r.settings,
  p.start_at AS program_start_at, p.end_at AS program_end_at, p.end_undecided AS program_end_undecided,
  p.name AS program_name, p.description AS program_description, p.extended AS program_extended,
  p.genres AS program_genres, p.related_items AS program_related_items
FROM reservations r
JOIN programs p ON p.service_id = r.service_id AND p.event_id = r.event_id
WHERE r.event_id IS NOT NULL AND coalesce(r.planned_end_at, r.end_at) > @now::timestamptz
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
INSERT INTO reservations (
  id, kind, service_id, event_id, start_at, end_at, name, description, extended, genres,
  enabled, follow_event_relay, settings, created_at, created_by, updated_at, updated_by, relayed_from
)
SELECT @id, 'program', p.service_id, p.event_id, p.start_at, p.end_at, p.name, p.description, p.extended, p.genres,
  true, true, @settings, @now, NULL, @now, NULL, @relayed_from
FROM programs p
WHERE p.service_id = @service_id AND p.event_id = @event_id
ON CONFLICT (service_id, event_id) WHERE event_id IS NOT NULL DO NOTHING
RETURNING id;
