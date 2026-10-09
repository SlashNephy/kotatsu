-- name: InsertRule :exec
INSERT INTO rules (
  id, name, memo, enabled, conditions, settings, valid_from, valid_until,
  avoid_duplicates, duplicate_window_days, follow_event_relay, created_at, created_by, updated_at, updated_by
)
VALUES (
  @id, @name, @memo, @enabled, @conditions, @settings, @valid_from, @valid_until,
  @avoid_duplicates, @duplicate_window_days, @follow_event_relay, @now, @actor, @now, @actor
);

-- name: GetRule :one
SELECT * FROM rules WHERE id = @id;

-- name: ListRules :many
SELECT * FROM rules ORDER BY created_at, id;

-- name: UpdateRule :execrows
UPDATE rules SET
  name = @name,
  memo = @memo,
  enabled = @enabled,
  conditions = @conditions,
  settings = @settings,
  valid_from = @valid_from,
  valid_until = @valid_until,
  avoid_duplicates = @avoid_duplicates,
  duplicate_window_days = @duplicate_window_days,
  follow_event_relay = @follow_event_relay,
  updated_at = @now,
  updated_by = @actor
WHERE id = @id;

-- name: DeleteRule :execrows
DELETE FROM rules WHERE id = @id;
