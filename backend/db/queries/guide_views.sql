-- name: ListGuideViews :many
SELECT * FROM guide_views WHERE user_id = @user_id ORDER BY created_at, id;

-- name: GetGuideView :one
SELECT * FROM guide_views WHERE id = @id AND user_id = @user_id;

-- name: InsertGuideView :exec
INSERT INTO guide_views (id, user_id, name, settings, hide_empty_services, created_at, updated_at)
VALUES (@id, @user_id, @name, @settings, @hide_empty_services, @now, @now);

-- name: UpdateGuideView :execrows
UPDATE guide_views SET name = @name, settings = @settings, hide_empty_services = @hide_empty_services, updated_at = @now
WHERE id = @id AND user_id = @user_id;

-- name: DeleteGuideView :execrows
DELETE FROM guide_views WHERE id = @id AND user_id = @user_id;
