-- name: UpsertJobProgress :exec
-- 動いているジョブの進み具合を書く (#1511)。試し直したら、前の試行の値を置き換える。
INSERT INTO job_progress (job_id, attempt, ratio, remaining_ms, updated_at)
VALUES (@job_id, @attempt::bigint, @ratio, sqlc.narg(remaining_ms), @now)
ON CONFLICT (job_id) DO UPDATE SET
  attempt = EXCLUDED.attempt, ratio = EXCLUDED.ratio, remaining_ms = EXCLUDED.remaining_ms, updated_at = EXCLUDED.updated_at;

-- name: DeleteJobProgressBefore :execrows
-- 書き直されなくなった進み具合 (終えたジョブのもの) を消す。
DELETE FROM job_progress WHERE updated_at < @before;

-- name: ListJobProgress :many
SELECT * FROM job_progress WHERE job_id = ANY(@job_ids::bigint[]);
