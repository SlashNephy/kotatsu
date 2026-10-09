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
