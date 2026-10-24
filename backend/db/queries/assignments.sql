-- name: ListAssignableReservations :many
-- 割り当ての対象の予約。有効で、録画の予定の終わりがまだ来ていないもの。
SELECT r.id, r.kind, r.start_at, r.end_at, r.settings, r.assignment_state, r.assigned_tuner,
  r.planned_start_at, r.planned_end_at, s.channel_type, s.channel
FROM reservations r
JOIN services s ON s.id = r.service_id
WHERE r.enabled AND coalesce(r.planned_end_at, r.end_at) > @now::timestamptz
ORDER BY r.id;

-- name: ListFirstRuleSettings :many
-- ルール予約の録画設定に使う、一致したルールのうち最も古いものの上書き。
SELECT DISTINCT ON (rr.reservation_id) rr.reservation_id, ru.settings
FROM reservation_rules rr
JOIN rules ru ON ru.id = rr.rule_id
WHERE rr.reservation_id = ANY(@reservation_ids::uuid[])
ORDER BY rr.reservation_id, ru.created_at, ru.id;

-- name: ListUsableTuners :many
SELECT tuner_index, types FROM tuners WHERE is_available AND NOT is_fault ORDER BY tuner_index;

-- name: UpdateAssignment :exec
UPDATE reservations SET
  assignment_state = @assignment_state,
  assigned_tuner = @assigned_tuner,
  planned_start_at = @planned_start_at,
  planned_end_at = @planned_end_at
WHERE id = @id;

-- name: ClearDisabledAssignments :execrows
-- 無効にした予約の割り当ての結果を消す。
UPDATE reservations SET assignment_state = 'pending', assigned_tuner = NULL, planned_start_at = NULL, planned_end_at = NULL
WHERE NOT enabled AND assignment_state <> 'pending';
