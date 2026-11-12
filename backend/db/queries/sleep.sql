-- name: GetSleepActivity :one
-- OS をスリープさせないかの判定に使う、録っている割り振りと、録画する予定の時刻に入っている予約の数 (#1522、設計書 11.7)。
-- 引き継ぎ待ちの割り振りも、古い mikan が録り続けているので、録っているとみなす。
-- 予約は、録画の割り振りを書く対象 (ListDueReservations) と同じく、有効で割り当てを計算済みのものを、
-- マージン込みで、開始マージンを前の録画に譲った後の、録画する予定の時刻で見る。
SELECT
  (SELECT count(*) FROM recording_assignments a WHERE a.state IN ('recording', 'handover'))::bigint AS recordings,
  (SELECT count(*) FROM timeshift_assignments t WHERE t.state IN ('recording', 'handover'))::bigint AS timeshifts,
  (
    SELECT count(*) FROM reservations r
    WHERE r.enabled AND r.assignment_state IN ('ok', 'partial', 'conflict')
      AND r.planned_start_at <= @now::timestamptz AND r.planned_end_at > @now::timestamptz
  )::bigint AS airing;

-- name: ListNextReservationStart :many
-- now より後に始まる、次の予約の録画する予定の開始 (#1522、設計書 11.7)。対象は GetSleepActivity と同じ。なければ空。
SELECT r.planned_start_at::timestamptz AS planned_start_at
FROM reservations r
WHERE r.enabled AND r.assignment_state IN ('ok', 'partial', 'conflict') AND r.planned_start_at > @now::timestamptz
ORDER BY r.planned_start_at
LIMIT 1;
