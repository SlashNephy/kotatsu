-- name: InsertUser :exec
INSERT INTO users (id, username, display_name, role, password_hash, disabled_at, created_at, updated_at)
VALUES (@id, @username, @display_name, @role, @password_hash, NULL, @created_at, @created_at);

-- name: GetUser :one
SELECT * FROM users WHERE id = @id;

-- name: DisableUser :execrows
UPDATE users SET disabled_at = @disabled_at, updated_at = @disabled_at WHERE id = @id AND disabled_at IS NULL;

-- name: ListUsersByIDs :many
SELECT * FROM users WHERE id = ANY(@ids::uuid[]);
