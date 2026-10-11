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

-- name: InsertRuleReservation :one
-- ルール予約を、番組情報をコピーして作る。同じ番組の予約がすでにあれば何もせず、行を返さない。
INSERT INTO reservations (
  id, kind, service_id, event_id, start_at, end_at, name, description, extended, genres,
  enabled, follow_event_relay, settings, created_at, created_by, updated_at, updated_by
)
SELECT @id, 'rule', p.service_id, p.event_id, p.start_at, p.end_at, p.name, p.description, p.extended, p.genres,
  true, @follow_event_relay, '{}', @now, NULL, @now, NULL
FROM programs p
WHERE p.service_id = @service_id AND p.event_id = @event_id
ON CONFLICT (service_id, event_id) WHERE event_id IS NOT NULL DO NOTHING
RETURNING id;

-- name: PromoteRuleReservation :execrows
-- ルール予約を、手動の番組予約に切り替え、手動の設定を書き込む (設計書 4.1)。無効にしたルール予約も、有効に戻す。
-- 一致したルールとの関連は残す。
UPDATE reservations
SET kind = 'program', enabled = true, follow_event_relay = @follow_event_relay, settings = @settings, updated_at = @now, updated_by = @updated_by
WHERE service_id = @service_id AND event_id = @event_id AND kind = 'rule';

-- name: SetReservationEnabled :execrows
UPDATE reservations SET enabled = @enabled, updated_at = @now, updated_by = @updated_by WHERE id = @id;

-- name: ListReservations :many
-- 時間帯 [from, to) に重なる予約を、開始時刻の順に返す。(after_start_at, after_id) より後ろから limit 件読む。
SELECT * FROM reservations
WHERE end_at > @from_at AND start_at < @to_at
  AND (cardinality(@states::text[]) = 0 OR assignment_state = ANY(@states::text[]))
  AND (sqlc.narg(enabled)::boolean IS NULL OR enabled = sqlc.narg(enabled)::boolean)
  AND (start_at, id) > (@after_start_at::timestamptz, @after_id::uuid)
ORDER BY start_at, id
LIMIT @row_limit;

-- name: ListReservationsByPrograms :many
-- 番組 (サービス, eventId) の予約。番組の予約は 1 件だけなので、番組ごとに高々 1 行になる。
SELECT * FROM reservations
WHERE (service_id, event_id) IN (SELECT unnest(@service_ids::bigint[]), unnest(@event_ids::bigint[]));

-- name: ListReservationRules :many
-- 予約に一致したルール。ルールを作った順に返す。
SELECT rr.reservation_id, rr.rule_id
FROM reservation_rules rr
JOIN rules ru ON ru.id = rr.rule_id
WHERE rr.reservation_id = ANY(@reservation_ids::uuid[])
ORDER BY rr.reservation_id, ru.created_at, ru.id;
