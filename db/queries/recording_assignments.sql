-- name: ListDueReservations :many
-- 割り振りを書く予約。有効で、チューナーの割り当てを計算済みで、予定の開始が due_before までに来て、まだ終わっておらず、割り振りのないもの。
SELECT r.id, r.assignment_state
FROM reservations r
WHERE r.enabled
  AND r.assignment_state IN ('ok', 'partial', 'conflict')
  AND r.planned_start_at <= @due_before::timestamptz
  AND r.planned_end_at > @now::timestamptz
  AND NOT EXISTS (SELECT 1 FROM recording_assignments a WHERE a.reservation_id = r.id)
ORDER BY r.planned_start_at, r.id;

-- name: InsertRecordingAssignment :exec
INSERT INTO recording_assignments (id, reservation_id, priority, state, generation, holder, lease_expires_at, created_at, updated_at)
VALUES (@id, @reservation_id, @priority, 'waiting', 0, NULL, NULL, @now, @now);

-- name: DeleteStaleRecordingAssignments :execrows
-- まだ mikan が取っていない割り振りのうち、録画しなくなった予約のものを消す。
-- 予約を無効にしたか、割り当てを計算し直す前の状態に戻ったか、予定の開始が due_before より後に動いたか、
-- mikan が取らないまま予定の終わりが過ぎたもの。
DELETE FROM recording_assignments a
USING reservations r
WHERE a.reservation_id = r.id
  AND a.state = 'waiting'
  AND (
    NOT r.enabled OR r.assignment_state = 'pending' OR r.planned_start_at > @due_before::timestamptz
    OR coalesce(r.planned_end_at, r.end_at) <= @now::timestamptz
  );

-- name: UpdateWaitingRecordingPriorities :execrows
-- まだ mikan が取っていない割り振りの優先度を、予約の今の割り当ての状態に合わせる。
UPDATE recording_assignments a
SET priority = CASE WHEN r.assignment_state = 'conflict' THEN @conflict_priority::integer ELSE @priority::integer END, updated_at = @now
FROM reservations r
WHERE a.reservation_id = r.id
  AND a.state = 'waiting'
  AND a.priority <> CASE WHEN r.assignment_state = 'conflict' THEN @conflict_priority::integer ELSE @priority::integer END;

-- name: ListRecordingAssignments :many
SELECT * FROM recording_assignments ORDER BY created_at, id;
