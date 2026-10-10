-- name: InsertUser :exec
INSERT INTO users (id, username, display_name, role, password_hash, disabled_at, created_at, updated_at)
VALUES (@id, @username, @display_name, @role, @password_hash, NULL, @created_at, @created_at);

-- name: GetUser :one
SELECT * FROM users WHERE id = @id;

-- name: DisableUser :execrows
UPDATE users SET disabled_at = @disabled_at, updated_at = @disabled_at WHERE id = @id AND disabled_at IS NULL;

-- name: ListUsersByIDs :many
SELECT * FROM users WHERE id = ANY(@ids::uuid[]);

-- name: ListUsers :many
SELECT * FROM users ORDER BY created_at, id;

-- name: GetUserForUpdate :one
SELECT * FROM users WHERE id = @id FOR UPDATE;

-- name: LockActiveAdmins :many
-- 有効な管理者の行をロックする。最後の管理者を外したり無効にしたりする操作を、同時に行わせない。
SELECT id FROM users WHERE role = 'admin' AND disabled_at IS NULL ORDER BY id FOR UPDATE;

-- name: UpdateUserProfile :one
UPDATE users SET display_name = @display_name, role = @role, updated_at = @now WHERE id = @id RETURNING *;

-- name: EnableUser :execrows
UPDATE users SET disabled_at = NULL, updated_at = @now WHERE id = @id AND disabled_at IS NOT NULL;
