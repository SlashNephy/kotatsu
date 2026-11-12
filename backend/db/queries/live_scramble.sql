-- name: UpsertLiveScrambleVerdict :exec
-- ライブで、サービス service_id がスクランブルされたまま届くと決めたことを記録する (設計書 7.1、#1517)。
-- 決める kyusu と tenban は別のホストのことがあり、時計がずれうる。後から決めたのに日時が戻ると、購読が新しい判定と見分けられないので、
-- 前の日時より必ず進める。
INSERT INTO live_scramble_verdicts (service_id, decided_at) VALUES (@service_id, @decided_at)
ON CONFLICT (service_id) DO UPDATE SET
  decided_at = GREATEST(EXCLUDED.decided_at, live_scramble_verdicts.decided_at + interval '1 microsecond');

-- name: ListLiveScrambleVerdicts :many
-- スクランブルされたまま届くと決めたサービスと、最後に決めた日時 (#1517)。サービスごとに 1 行なので、サービスの数より増えない。
SELECT service_id, decided_at FROM live_scramble_verdicts ORDER BY service_id;
