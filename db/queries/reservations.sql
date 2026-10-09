-- name: GetProgramForReservation :one
SELECT service_id, event_id, start_at, end_at, name, description, extended, genres
FROM programs
WHERE service_id = @service_id AND event_id = @event_id;

-- name: InsertProgramReservation :one
-- 同じ番組の予約がすでにあれば何もしない。そのときは行を返さない。
INSERT INTO reservations (
  id, kind, service_id, event_id, start_at, end_at, name, description, extended, genres,
  enabled, follow_event_relay, settings, created_at, created_by, updated_at, updated_by
)
VALUES (
  @id, 'program', @service_id, @event_id, @start_at, @end_at, @name, @description, @extended, @genres,
  true, @follow_event_relay, @settings, @now, @created_by, @now, @created_by
)
ON CONFLICT (service_id, event_id) WHERE event_id IS NOT NULL DO NOTHING
RETURNING id;

-- name: InsertTimeReservation :exec
INSERT INTO reservations (
  id, kind, service_id, event_id, start_at, end_at, name, description, extended, genres,
  enabled, follow_event_relay, settings, created_at, created_by, updated_at, updated_by
)
VALUES (
  @id, 'time', @service_id, NULL, @start_at, @end_at, @name, '', '{}', '[]',
  true, false, @settings, @now, @created_by, @now, @created_by
);

-- name: GetReservation :one
SELECT * FROM reservations WHERE id = @id;

-- name: GetReservationByProgram :one
SELECT * FROM reservations WHERE service_id = @service_id AND event_id = @event_id;

-- name: UpdateReservation :execrows
UPDATE reservations SET
  start_at = @start_at,
  end_at = @end_at,
  name = @name,
  enabled = @enabled,
  follow_event_relay = @follow_event_relay,
  settings = @settings,
  updated_at = @now,
  updated_by = @updated_by
WHERE id = @id;

-- name: DeleteReservation :execrows
DELETE FROM reservations WHERE id = @id;
