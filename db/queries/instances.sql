-- name: UpsertInstance :exec
INSERT INTO instances (id, role, address, started_at, heartbeat_at)
VALUES (@id, @role, @address, @now, @now)
ON CONFLICT (id, role) DO UPDATE
SET address = EXCLUDED.address, heartbeat_at = EXCLUDED.heartbeat_at;

-- name: HeartbeatInstance :execrows
UPDATE instances SET heartbeat_at = @now WHERE id = @id AND role = @role;

-- name: ListAliveInstances :many
SELECT * FROM instances
WHERE role = @role AND heartbeat_at > @alive_since
ORDER BY started_at, id;

-- name: DeleteInstance :exec
DELETE FROM instances WHERE id = @id AND role = @role;

-- name: DeleteStaleInstances :execrows
DELETE FROM instances WHERE heartbeat_at < @before;
