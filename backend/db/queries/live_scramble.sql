-- name: UpsertLiveScrambleVerdict :exec
-- ライブで、サービス service_id がスクランブルされたまま届くと決めたことを記録する (設計書 7.1、#1517)。
-- 決める kyusu と tenban は別のホストのことがあり、時計がずれうる。後から決めたのに日時が戻ると、購読が新しい判定と見分けられないので、
-- 前の日時より必ず進める。
INSERT INTO live_scramble_verdicts (service_id, decided_at) VALUES (@service_id, @decided_at)
ON CONFLICT (service_id) DO UPDATE SET
  decided_at = GREATEST(EXCLUDED.decided_at, live_scramble_verdicts.decided_at + interval '1 microsecond');

-- name: GetLiveScrambleVerdict :one
-- サービス service_id が、最後にスクランブルされたまま届くと決めた日時。決めたことがなければ行を返さない。
SELECT decided_at FROM live_scramble_verdicts WHERE service_id = @service_id;
