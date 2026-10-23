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
-- 引き継ぎ待ちの割り振りは、リースが切れていなくても、ほかの mikan が取る (設計書 5.4)。
-- 取る前の状態と、リースの期限も返す。引き継ぎか、落ちた mikan の代わりかで、途切れた区間の扱いが変わる。
WITH target AS (
  SELECT a.id, a.state AS previous_state, a.lease_expires_at AS previous_lease_expires_at FROM recording_assignments a
  WHERE (a.state IN ('waiting', 'recording') AND (a.holder IS NULL OR a.lease_expires_at <= @now::timestamptz))
    OR (a.state = 'handover' AND a.holder IS DISTINCT FROM @holder::uuid)
  ORDER BY a.priority DESC, a.created_at, a.id
  LIMIT 1
  FOR UPDATE SKIP LOCKED
)
UPDATE recording_assignments a
SET holder = @holder, generation = a.generation + 1, lease_expires_at = @expires_at, state = 'recording', updated_at = @now
FROM target
WHERE a.id = target.id
RETURNING a.id, a.reservation_id, a.priority, a.generation, target.previous_state, target.previous_lease_expires_at;

-- name: RenewRecordingAssignment :execrows
-- 割り振りのリースを延長する。世代番号が進んでいたら、ほかの mikan が取ったので、延長しない。引き継ぎ待ちの間も延長する。
UPDATE recording_assignments SET lease_expires_at = @expires_at, updated_at = @now
WHERE id = @id AND generation = @generation AND holder = @holder AND state IN ('recording', 'handover') AND lease_expires_at > @now::timestamptz;

-- name: RecordingAssignmentExists :one
-- 割り振りが残っているかどうか。予約を消すと、割り振りも一緒に消える。
SELECT EXISTS (SELECT 1 FROM recording_assignments WHERE id = @id);

-- name: MarkRecordingHandover :execrows
-- 止まる mikan が、割り振りに引き継ぎ待ちの印を付ける (設計書 5.4)。
UPDATE recording_assignments SET state = 'handover', updated_at = @now
WHERE id = @id AND generation = @generation AND holder = @holder AND state = 'recording';

-- name: ReleaseRecordingAssignment :exec
-- 止まる mikan が、引き継ぐ mikan がいないときに、割り振りを手放す。再起動した mikan が、リースが切れるのを待たずに取れる。
UPDATE recording_assignments SET state = 'recording', holder = NULL, lease_expires_at = NULL, updated_at = @now
WHERE id = @id AND generation = @generation AND holder = @holder AND state IN ('recording', 'handover');

-- name: MarkRecordingStreamOpened :exec
-- 割り振りを取った mikan が、ストリームを開いたことを記録する。
UPDATE recording_assignments SET stream_generation = @generation, updated_at = @now
WHERE id = @id AND generation = @generation AND stream_generation < @generation;

-- name: GetRecordingAssignmentStream :one
SELECT generation, stream_generation FROM recording_assignments WHERE id = @id;

-- name: FinishRecordingAssignment :execrows
-- 録画を終えた割り振りを、終わったことにする。世代番号が進んでいたら、何もしない。引き継ぎ待ちの間に予定の終わりが来ても終える。
UPDATE recording_assignments SET state = 'finished', holder = NULL, lease_expires_at = NULL, updated_at = @now
WHERE id = @id AND generation = @generation AND holder = @holder AND state IN ('recording', 'handover');

-- name: GetReservationForRecording :one
-- 録画に使う予約の情報。予定の時刻は、録画中も neko が計算し直すので、録画しながら読み直す。
SELECT r.id, r.kind, r.service_id, r.event_id, r.name, r.description, r.start_at, r.end_at, r.enabled,
  r.planned_start_at, r.planned_end_at, r.settings, s.name AS service_name, s.channel_type, s.channel
FROM reservations r
JOIN services s ON s.id = r.service_id
WHERE r.id = @id;

-- name: LockRecordingAssignmentGeneration :one
-- 録画を終える前に、割り振りをロックして、今の世代番号を読む。ほかの mikan が取っていたら、録画を終えない。
SELECT generation FROM recording_assignments WHERE id = @id FOR UPDATE;

-- name: ListMissedRecordingAssignments :many
-- mikan が 1 台も取らないまま、予定の終わりが過ぎた割り振りの予約。録画の行がまだなければ、失敗として記録する (#517)。
SELECT r.id, r.service_id, r.event_id, r.name, r.description, r.start_at, r.end_at, r.planned_start_at, s.name AS service_name
FROM recording_assignments a
JOIN reservations r ON r.id = a.reservation_id
JOIN services s ON s.id = r.service_id
WHERE a.state = 'waiting'
  AND r.enabled
  AND coalesce(r.planned_end_at, r.end_at) <= @now::timestamptz
  AND NOT EXISTS (SELECT 1 FROM recordings rec WHERE rec.reservation_id = r.id)
ORDER BY r.id;

-- name: DeleteUnrecordedRecordingAssignment :execrows
-- 録画の行を作らずに終えた割り振り (録画を始める前に予約を無効にした、など) を消す。世代番号が進んでいたら、何もしない。
-- 終わったことにして残すと、予約を有効に戻しても、割り振りを書き直せない (#523)。
DELETE FROM recording_assignments a
WHERE a.id = @id AND a.generation = @generation AND a.holder = @holder AND a.state IN ('recording', 'handover')
  AND NOT EXISTS (SELECT 1 FROM recordings rec WHERE rec.reservation_id = a.reservation_id);

-- name: ReleaseUnrecordableRecordingAssignment :exec
-- 録画できない理由がこの mikan にあるので、割り振りを手放し、まだ取っていない状態に戻す (#524)。ほかの mikan が取る。
-- まだ何も録っていないので、続きとしてではなく、初めから録る。予定の終わりまで誰も取らなければ、neko が消す。
UPDATE recording_assignments SET state = 'waiting', holder = NULL, lease_expires_at = NULL, updated_at = @now
WHERE id = @id AND generation = @generation AND holder = @holder AND state = 'recording';

-- name: GetDatabaseNow :one
-- DB の今の時刻。mikan の時計がずれていても、割り振りのリースの期限を同じ時刻で比べるのに使う (#526)。
SELECT clock_timestamp()::timestamptz AS now;
