-- name: InsertAuditEvent :exec
INSERT INTO audit_events (
  id, occurred_at, actor_type, actor_user_id, actor_role, action, target_type, target_id, summary, source_ip, user_agent
) VALUES (
  @id, @occurred_at, @actor_type, @actor_user_id, @actor_role, @action, @target_type, @target_id, @summary, @source_ip, @user_agent
);

-- name: DeleteAuditEventsBefore :execrows
DELETE FROM audit_events WHERE occurred_at < @before;

-- name: ListAuditEvents :many
SELECT * FROM audit_events ORDER BY occurred_at DESC, id DESC LIMIT @max_count;

-- name: SearchAuditEvents :many
-- 監査ログを、新しい順に返す。条件の NULL の項目は絞り込まない。
-- before_at と before_id より前 (古い) の記録だけを返し、ページを送る。
SELECT * FROM audit_events
WHERE (sqlc.narg(action_prefix)::text IS NULL OR starts_with(action, sqlc.narg(action_prefix)::text))
  AND (sqlc.narg(actor_user_id)::uuid IS NULL OR actor_user_id = sqlc.narg(actor_user_id)::uuid)
  AND (sqlc.narg(target_type)::text IS NULL OR target_type = sqlc.narg(target_type)::text)
  AND (sqlc.narg(target_id)::text IS NULL OR target_id = sqlc.narg(target_id)::text)
  AND (sqlc.narg(since)::timestamptz IS NULL OR occurred_at >= sqlc.narg(since)::timestamptz)
  AND (sqlc.narg(until)::timestamptz IS NULL OR occurred_at < sqlc.narg(until)::timestamptz)
  AND (sqlc.narg(before_at)::timestamptz IS NULL OR (occurred_at, id) < (sqlc.narg(before_at)::timestamptz, sqlc.narg(before_id)::uuid))
ORDER BY occurred_at DESC, id DESC
LIMIT @max_count;
