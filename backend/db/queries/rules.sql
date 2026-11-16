-- name: InsertRule :exec
INSERT INTO rules (
  id, name, memo, enabled, conditions, settings, valid_from, valid_until,
  avoid_duplicates, duplicate_window_days, follow_event_relay, bundle_same_episodes, skip_earlier_episodes, group_id,
  created_at, created_by, updated_at, updated_by
)
VALUES (
  @id, @name, @memo, @enabled, @conditions, @settings, @valid_from, @valid_until,
  @avoid_duplicates, @duplicate_window_days, @follow_event_relay, @bundle_same_episodes, @skip_earlier_episodes, sqlc.narg(group_id),
  @now, @actor, @now, @actor
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
  bundle_same_episodes = @bundle_same_episodes,
  skip_earlier_episodes = @skip_earlier_episodes,
  -- keep_group なら、グループを変えずに残す (#1493)。API で group_id を省いた変更が、黙ってグループを外さないためである。
  group_id = CASE WHEN @keep_group::boolean THEN group_id ELSE sqlc.narg(group_id)::uuid END,
  updated_at = @now,
  updated_by = @actor
WHERE id = @id;

-- name: DeleteRule :execrows
DELETE FROM rules WHERE id = @id;

-- name: SetRuleEnabled :execrows
UPDATE rules SET enabled = @enabled, updated_at = @now, updated_by = @actor WHERE id = @id;

-- name: ListRulesByIDs :many
SELECT * FROM rules WHERE id = ANY(@ids::uuid[]);

-- name: SetRuleGroup :execrows
-- ルールのグループを変える (#1493)。照合には関わらないので、updated_at は変えない。変えた日での並べ替えを、まとめて移しただけで崩さないためである。
UPDATE rules SET group_id = sqlc.narg(group_id) WHERE id = @id;

-- name: InsertRuleGroup :exec
-- 新しいグループは、一覧の最後 (今のグループの後) に並べる。
INSERT INTO rule_groups (id, name, position, created_at, created_by, updated_at, updated_by)
VALUES (@id, @name, (SELECT coalesce(max(position) + 1, 0) FROM rule_groups), @now, @actor, @now, @actor);

-- name: GetRuleGroup :one
SELECT * FROM rule_groups WHERE id = @id;

-- name: ListRuleGroups :many
SELECT * FROM rule_groups ORDER BY position, created_at, id;

-- name: ListRuleGroupsByIDs :many
SELECT * FROM rule_groups WHERE id = ANY(@ids::uuid[]);

-- name: RenameRuleGroup :execrows
UPDATE rule_groups SET name = @name, updated_at = @now, updated_by = @actor WHERE id = @id;

-- name: ReorderRuleGroups :exec
-- グループを ids の順に並べ、position を 0 から振り直す。
UPDATE rule_groups AS g SET position = o.ord - 1
FROM unnest(@ids::uuid[]) WITH ORDINALITY AS o (id, ord)
WHERE g.id = o.id;

-- name: CountRulesInGroup :one
SELECT count(*) FROM rules WHERE group_id = @group_id;

-- name: DeleteRuleGroup :execrows
-- 中のルールは、外部キーの ON DELETE SET NULL で「その他」に戻る。
DELETE FROM rule_groups WHERE id = @id;

-- name: LockRuleGroups :exec
-- グループの並びを変えるときと足すときに、ほかの並べ替えと足す操作を待たせ、並び順の重なりと抜けを防ぐ。
LOCK TABLE rule_groups IN SHARE ROW EXCLUSIVE MODE;
