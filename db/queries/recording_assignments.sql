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

-- name: AcquireRecordingAssignment :one
-- mikan が、誰も持っていないか、リースの切れた割り振りを 1 つ取る。ほかの mikan が同時に取ろうとしている行は飛ばす。
-- 取るたびに世代番号を上げ、古い mikan のリースの延長を失敗させる。優先度の高い録画から取る。
WITH target AS (
  SELECT a.id FROM recording_assignments a
  WHERE a.state IN ('waiting', 'recording') AND (a.holder IS NULL OR a.lease_expires_at <= @now::timestamptz)
  ORDER BY a.priority DESC, a.created_at, a.id
  LIMIT 1
  FOR UPDATE SKIP LOCKED
)
UPDATE recording_assignments a
SET holder = @holder, generation = a.generation + 1, lease_expires_at = @expires_at, state = 'recording', updated_at = @now
FROM target
WHERE a.id = target.id
RETURNING a.*;

-- name: RenewRecordingAssignment :execrows
-- 割り振りのリースを延長する。世代番号が進んでいたら、ほかの mikan が取ったので、延長しない。
UPDATE recording_assignments SET lease_expires_at = @expires_at, updated_at = @now
WHERE id = @id AND generation = @generation AND holder = @holder AND state = 'recording' AND lease_expires_at > @now::timestamptz;

-- name: FinishRecordingAssignment :execrows
-- 録画を終えた割り振りを、終わったことにする。世代番号が進んでいたら、何もしない。
UPDATE recording_assignments SET state = 'finished', holder = NULL, lease_expires_at = NULL, updated_at = @now
WHERE id = @id AND generation = @generation AND holder = @holder;

-- name: GetReservationForRecording :one
-- 録画に使う予約の情報。予定の時刻は、録画中も neko が計算し直すので、録画しながら読み直す。
SELECT r.id, r.kind, r.service_id, r.event_id, r.name, r.description, r.start_at, r.end_at, r.enabled,
  r.planned_start_at, r.planned_end_at, r.settings, s.name AS service_name, s.channel_type, s.channel
FROM reservations r
JOIN services s ON s.id = r.service_id
WHERE r.id = @id;
