-- name: ListTimeshiftChannels :many
-- タイムシフトで録るチャンネル。割り振りと、録画のチューナーとの衝突の判定には、サービスのチャンネル種別とチャンネルが要る。
-- チューナーが足りないときに先に録るものを決めるため、チャンネル種別の並び順、サービス ID の順に返す。
SELECT t.service_id, t.retention_seconds, t.created_at, t.updated_at, t.updated_by, s.name AS service_name, s.channel_type, s.channel
FROM timeshift_channels t
JOIN services s ON s.id = t.service_id
JOIN channel_types ct ON ct.type = s.channel_type
ORDER BY ct.sort_order, t.service_id;

-- name: UpsertTimeshiftChannel :exec
INSERT INTO timeshift_channels (service_id, retention_seconds, created_at, updated_at, updated_by)
VALUES (@service_id, @retention_seconds, @now, @now, @updated_by)
ON CONFLICT (service_id) DO UPDATE
SET retention_seconds = EXCLUDED.retention_seconds, updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by;

-- name: DeleteTimeshiftChannel :execrows
DELETE FROM timeshift_channels WHERE service_id = @service_id;

-- name: ListTimeshiftAssignments :many
SELECT * FROM timeshift_assignments ORDER BY created_at, id;

-- name: InsertTimeshiftAssignment :exec
INSERT INTO timeshift_assignments (id, service_id, priority, state, generation, holder, lease_expires_at, created_at, updated_at)
VALUES (@id, @service_id, @priority, 'waiting', 0, NULL, NULL, @now, @now);

-- name: DeleteTimeshiftAssignment :execrows
-- neko が、チューナーを渡すため、またはチャンネルの設定がなくなったために、割り振りを取り消す。状態によらず消す。
DELETE FROM timeshift_assignments WHERE service_id = @service_id;

-- name: AcquireTimeshiftAssignment :one
-- mikan が、誰も持っていないか、リースの切れたタイムシフトの割り振りを 1 つ取る。ほかの mikan が同時に取ろうとしている行は飛ばす。
-- 取るたびに世代番号を上げ、古い mikan のリースの延長を失敗させる。優先度の高いものから取る。
-- 引き継ぎ待ちの割り振りは、リースが切れていなくても、ほかの mikan が取る (設計書 5.4)。
-- 取る前の状態と、リースの期限も返す。引き継ぎか、落ちた mikan の代わりかで、途切れた区間の扱いが変わる。
WITH target AS (
  SELECT a.id, a.state AS previous_state, a.lease_expires_at AS previous_lease_expires_at FROM timeshift_assignments a
  WHERE (a.state IN ('waiting', 'recording') AND (a.holder IS NULL OR a.lease_expires_at <= @now::timestamptz))
    OR (a.state = 'handover' AND a.holder IS DISTINCT FROM @holder::uuid)
  ORDER BY a.priority DESC, a.created_at, a.id
  LIMIT 1
  FOR UPDATE SKIP LOCKED
)
UPDATE timeshift_assignments a
SET holder = @holder, generation = a.generation + 1, lease_expires_at = @expires_at, state = 'recording', updated_at = @now
FROM target
WHERE a.id = target.id
RETURNING a.id, a.service_id, a.priority, a.generation, target.previous_state, target.previous_lease_expires_at;

-- name: RenewTimeshiftAssignment :execrows
-- 割り振りのリースを延長する。世代番号が進んでいたら、ほかの mikan が取ったので、延長しない。引き継ぎ待ちの間も延長する。
UPDATE timeshift_assignments SET lease_expires_at = @expires_at, updated_at = @now
WHERE id = @id AND generation = @generation AND holder = @holder AND state IN ('recording', 'handover') AND lease_expires_at > @now::timestamptz;

-- name: TimeshiftAssignmentExists :one
-- 割り振りが残っているかどうか。neko が取り消すと、割り振りは消える。
SELECT EXISTS (SELECT 1 FROM timeshift_assignments WHERE id = @id);

-- name: MarkTimeshiftHandover :execrows
-- 止まる mikan が、割り振りに引き継ぎ待ちの印を付ける (設計書 5.4)。
UPDATE timeshift_assignments SET state = 'handover', updated_at = @now
WHERE id = @id AND generation = @generation AND holder = @holder AND state = 'recording';

-- name: ReleaseTimeshiftAssignment :exec
-- 止まる mikan が、引き継ぐ mikan がいないときに、割り振りを手放す。再起動した mikan が、リースが切れるのを待たずに取れる。
UPDATE timeshift_assignments SET state = 'recording', holder = NULL, lease_expires_at = NULL, updated_at = @now
WHERE id = @id AND generation = @generation AND holder = @holder AND state IN ('recording', 'handover');

-- name: MarkTimeshiftStreamOpened :exec
-- 割り振りを取った mikan が、ストリームを開いたことを記録する。
UPDATE timeshift_assignments SET stream_generation = @generation, updated_at = @now
WHERE id = @id AND generation = @generation AND stream_generation < @generation;

-- name: GetTimeshiftAssignmentStream :one
SELECT generation, stream_generation FROM timeshift_assignments WHERE id = @id;

-- name: FinishTimeshiftAssignment :execrows
-- タイムシフトを終えた割り振りを、終わったことにする。世代番号が進んでいたら、何もしない。
UPDATE timeshift_assignments SET state = 'finished', holder = NULL, lease_expires_at = NULL, updated_at = @now
WHERE id = @id AND generation = @generation AND holder = @holder AND state IN ('recording', 'handover');

-- name: LockTimeshiftAssignmentGeneration :one
-- 割り振りを手放す前に、行をロックして、今の世代番号を読む。ほかの mikan が取っていたら、手放さない。
SELECT generation FROM timeshift_assignments WHERE id = @id FOR UPDATE;

-- name: DeleteTimeshiftAssignmentByGeneration :execrows
-- 割り振りを終えるために消す。タイムシフトには録画の行がないので、終えることは消すこと。録り続けるべきチャンネルなら、neko が次の周期で書き直す。
-- 世代番号が進んでいたら、何もしない。
DELETE FROM timeshift_assignments
WHERE id = @id AND generation = @generation AND holder = @holder AND state IN ('recording', 'handover');

-- name: ReleaseUnrecordableTimeshiftAssignment :exec
-- 録れない理由がこの mikan にあるので、割り振りを手放し、まだ取っていない状態に戻す。ほかの mikan が取る。
UPDATE timeshift_assignments SET state = 'waiting', holder = NULL, lease_expires_at = NULL, updated_at = @now
WHERE id = @id AND generation = @generation AND holder = @holder AND state = 'recording';

-- name: InsertTimeshiftChunk :exec
INSERT INTO timeshift_chunks (id, service_id, path, size, started_at, ended_at, created_at)
VALUES (@id, @service_id, @path, @size, @started_at, NULL, @now);

-- name: CloseTimeshiftChunk :execrows
-- 書き終えたチャンクの大きさと終わりの時刻を記録する。
UPDATE timeshift_chunks SET size = @size, ended_at = @ended_at
WHERE id = @id;

-- name: CloseStaleTimeshiftChunks :execrows
-- mikan が落ちるなどして、書きかけのまま残ったチャンクを閉じる。サービスを録り直す前に呼ぶ。
UPDATE timeshift_chunks SET ended_at = @ended_at
WHERE service_id = @service_id AND ended_at IS NULL;

-- name: ListExpiredTimeshiftChunks :many
-- 破棄するチャンク。書いている途中のものは除く。チャンネルの設定がなくなったサービスのものは全部、設定があるものは保持時間を過ぎたもの。
SELECT c.id, c.service_id, c.path
FROM timeshift_chunks c
LEFT JOIN timeshift_channels t ON t.service_id = c.service_id
WHERE c.ended_at IS NOT NULL
  AND (t.service_id IS NULL OR c.ended_at + t.retention_seconds * interval '1 second' < @now::timestamptz)
ORDER BY c.started_at, c.id;

-- name: DeleteTimeshiftChunk :execrows
DELETE FROM timeshift_chunks WHERE id = @id;

-- name: ListTimeshiftChunkIDs :many
-- 記録のあるチャンクの ID とパス。ディスク上のファイルとの突き合わせに使う。
SELECT id, path FROM timeshift_chunks ORDER BY started_at, id;

-- name: StartTimeshiftProgram :exec
-- 番組の始まりを記録する。同じ時刻の行があれば、番組の情報を更新する。
INSERT INTO timeshift_programs (service_id, started_at, event_id, name, ended_at)
VALUES (@service_id, @started_at, @event_id, @name, NULL)
ON CONFLICT (service_id, started_at) DO UPDATE SET event_id = EXCLUDED.event_id, name = EXCLUDED.name;

-- name: EndTimeshiftProgram :execrows
-- 終わっていない番組を閉じる。
UPDATE timeshift_programs SET ended_at = @ended_at
WHERE service_id = @service_id AND ended_at IS NULL;

-- name: DeleteTimeshiftProgramsBefore :execrows
-- 終わりが before より前の番組を消す。チャンクの破棄に合わせて呼ぶ。
DELETE FROM timeshift_programs
WHERE service_id = @service_id AND ended_at IS NOT NULL AND ended_at < @before::timestamptz;

-- name: ListReservationsUsingTunersBetween :many
-- from から to までの間にチューナーを使う予約。有効で、チューナーを割り当て済みで、予定が期間と重なるもの。録画中の予約も含む。
SELECT r.id, r.assigned_tuner, s.channel_type, s.channel
FROM reservations r
JOIN services s ON s.id = r.service_id
WHERE r.enabled
  AND r.assigned_tuner IS NOT NULL
  AND r.planned_start_at < @to_at::timestamptz
  AND r.planned_end_at > @from_at::timestamptz
ORDER BY r.planned_start_at, r.id;
