-- name: ListTimeshiftChannels :many
-- タイムシフトで録るチャンネル。割り振りと、録画のチューナーとの衝突の判定には、サービスのチャンネル種別とチャンネルが要る。
-- チューナーが足りないときに先に録るものを決めるため、チャンネル種別の並び順、サービス ID の順に返す。
SELECT t.service_id, t.retention_seconds, t.created_at, t.updated_at, t.updated_by, s.name AS service_name, s.channel_type, s.channel
FROM timeshift_channels t
JOIN services s ON s.id = t.service_id
JOIN channel_types ct ON ct.type = s.channel_type
ORDER BY ct.sort_order, t.service_id;

-- name: ListServiceChannels :many
-- サービスのあるチャンネル種別とチャンネルの組。チューナーサーバーが番組表を取るチャンネルで、
-- タイムシフトの割り振りで、番組表の取得に残すチューナーを決めるのに使う。
SELECT DISTINCT channel_type, channel FROM services ORDER BY channel_type, channel;

-- name: UpsertTimeshiftChannel :exec
INSERT INTO timeshift_channels (service_id, retention_seconds, created_at, updated_at, updated_by)
VALUES (@service_id, @retention_seconds, @now, @now, @updated_by)
ON CONFLICT (service_id) DO UPDATE
SET retention_seconds = EXCLUDED.retention_seconds, updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by;

-- name: DeleteTimeshiftChannel :execrows
DELETE FROM timeshift_channels WHERE service_id = @service_id;

-- name: LockTimeshiftChannel :one
-- 変える前の保持時間を監査ログに残すため、チャンネルの行をロックして読む。
SELECT retention_seconds FROM timeshift_channels WHERE service_id = @service_id FOR UPDATE;

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
  WHERE ((a.state IN ('waiting', 'recording') AND (a.holder IS NULL OR a.lease_expires_at <= @now::timestamptz))
    OR (a.state = 'handover' AND a.holder IS DISTINCT FROM @holder::uuid))
    -- 自分が録っている割り振りは取らない。DB が止まっていた間にリースの期限が切れても、取り直すと世代番号が進み、録画が止まる (#1073)。
    AND NOT (a.id = ANY(@active::uuid[]))
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
-- 期限が切れていても、世代番号が進んでいなければ誰も取っていないので、延長する。DB が止まっていた間に期限が切れても、録り続けるため (#1073)。
UPDATE timeshift_assignments SET lease_expires_at = @expires_at, updated_at = @now
WHERE id = @id AND generation = @generation AND holder = @holder AND state IN ('recording', 'handover');

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

-- name: ListOpenTimeshiftChunks :many
-- 書きかけのチャンク。mikan が落ちるなどして残ったものを、サービスを録り直す前か、外したチャンネルのものを破棄する前に、
-- ファイルの大きさと最後に書いた時刻で閉じる。
SELECT id, path, started_at FROM timeshift_chunks
WHERE service_id = @service_id AND ended_at IS NULL
ORDER BY started_at, id;

-- name: ListAbandonedTimeshiftServices :many
-- タイムシフトから外したチャンネルのうち、書きかけのチャンクか、放送中のままの番組が残り、割り振りのないサービス。
-- mikan が強制終了した直後に外すと、閉じる mikan がいなくなるので、破棄のジョブが閉じる。
-- 割り振りが残っていれば、mikan がまだ書いているかもしれないので、含めない。neko が割り振りを消せば、mikan が自分で閉じる。
SELECT s.service_id FROM (
  SELECT c.service_id FROM timeshift_chunks c WHERE c.ended_at IS NULL
  UNION
  SELECT p.service_id FROM timeshift_programs p WHERE p.ended_at IS NULL
) s
WHERE NOT EXISTS (SELECT 1 FROM timeshift_channels t WHERE t.service_id = s.service_id)
  AND NOT EXISTS (SELECT 1 FROM timeshift_assignments a WHERE a.service_id = s.service_id)
ORDER BY s.service_id;

-- name: GetLastTimeshiftChunkEnd :one
-- サービスのチャンクの、最も遅い終わり。落ちた mikan が残した放送中の番組を、録れていた最後の時刻で閉じるのに使う。
SELECT ended_at FROM timeshift_chunks
WHERE service_id = @service_id AND ended_at IS NOT NULL
ORDER BY ended_at DESC
LIMIT 1;

-- name: ListExpiredTimeshiftChunks :many
-- 破棄するチャンク。書いている途中のものは除く。チャンネルの設定がなくなったサービスのものは全部、設定があるものは保持時間を過ぎたもの。
-- 設定がなくなったサービスの、録る mikan のいない書きかけのチャンクは、先に閉じてから破棄する (ListAbandonedTimeshiftServices)。
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
-- 番組の始まりを記録する。同じ時刻の行があれば、その番組は始まってすぐに替わったので、新しい番組で置き換え、放送中に戻す。
INSERT INTO timeshift_programs (service_id, started_at, event_id, name, ended_at)
VALUES (@service_id, @started_at, @event_id, @name, NULL)
ON CONFLICT (service_id, started_at) DO UPDATE SET event_id = EXCLUDED.event_id, name = EXCLUDED.name, ended_at = NULL;

-- name: GetOpenTimeshiftProgram :one
-- 放送中として記録している番組。引き継いだ mikan が、同じ番組を 2 つの行に分けないよう、続きから追うのに使う。
-- 落ちた mikan が残した番組を閉じるとき、チャンクがなければ、始まりで閉じるのにも使う。
SELECT event_id, started_at FROM timeshift_programs
WHERE service_id = @service_id AND ended_at IS NULL
ORDER BY started_at DESC
LIMIT 1;

-- name: EndTimeshiftProgram :execrows
-- 終わっていない番組を閉じる。
UPDATE timeshift_programs SET ended_at = @ended_at
WHERE service_id = @service_id AND ended_at IS NULL;

-- name: DeleteStaleTimeshiftPrograms :execrows
-- 再生できる範囲の外になった、終わった番組を、全サービスでまとめて消す。チャンクの破棄に合わせて呼ぶ。
-- 範囲の外は、サービスの残っているチャンクの最も早い始まりより前。チャンクが 1 つも残っていなければ、今より前。
-- タイムシフトから外し、チャンクのなくなったサービスの行も残さない。
DELETE FROM timeshift_programs p
WHERE p.ended_at IS NOT NULL
  AND p.ended_at < coalesce(
    (SELECT min(c.started_at) FROM timeshift_chunks c WHERE c.service_id = p.service_id),
    @now::timestamptz
  );

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

-- name: ListTimeshiftChannelStatuses :many
-- タイムシフトで録るチャンネルと、mikan が録っているか。管理の画面に使う。
-- 録っているかは、mikan が割り振りを持ち、リースが切れていないかで決める。
SELECT t.service_id, t.retention_seconds,
  EXISTS (
    SELECT 1 FROM timeshift_assignments a
    WHERE a.service_id = t.service_id AND a.state IN ('recording', 'handover')
      AND a.holder IS NOT NULL AND a.lease_expires_at > @now::timestamptz
  ) AS recording
FROM timeshift_channels t
JOIN services s ON s.id = t.service_id
JOIN channel_types ct ON ct.type = s.channel_type
ORDER BY ct.sort_order, t.service_id;

-- name: ListTimeshiftBufferRanges :many
-- タイムシフトで録るチャンネルの、バッファにある範囲。管理の画面とメトリクスに使う。チャンクのないサービスは返さない。
-- 範囲は、最も古いチャンクの始まりから、最新のチャンクの終わりまで。書いている途中のチャンクは、今まで録れているものとする。
SELECT c.service_id,
  min(c.started_at)::timestamptz AS buffered_from,
  max(coalesce(c.ended_at, @now::timestamptz))::timestamptz AS buffered_until
FROM timeshift_chunks c
JOIN timeshift_channels t ON t.service_id = c.service_id
GROUP BY c.service_id
ORDER BY c.service_id;
