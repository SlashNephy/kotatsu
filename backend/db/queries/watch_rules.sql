-- name: InsertWatchRule :exec
INSERT INTO watch_rules (id, user_id, name, enabled, conditions, created_at, updated_at)
VALUES (@id, @user_id, @name, @enabled, @conditions, @now, @now);

-- name: LockWatchRules :exec
-- ユーザーの視聴ルールの数を数えてから作るまでの間に、同じユーザーがほかの端末で作らないよう、トランザクションの間だけロックを取る。
SELECT pg_advisory_xact_lock(hashtext('kotatsu.watch_rules'), hashtext((@user_id::uuid)::text));

-- name: CountWatchRules :one
SELECT count(*) FROM watch_rules WHERE user_id = @user_id;

-- name: GetWatchRule :one
-- ほかのユーザーの視聴ルールは返さない。
SELECT * FROM watch_rules WHERE id = @id AND user_id = @user_id;

-- name: ListWatchRules :many
SELECT * FROM watch_rules WHERE user_id = @user_id ORDER BY created_at, id;

-- name: ListEnabledWatchRules :many
-- 照合に使う、すべてのユーザーの有効な視聴ルール。
SELECT * FROM watch_rules WHERE enabled ORDER BY user_id, created_at, id;

-- name: UpdateWatchRule :execrows
UPDATE watch_rules SET name = @name, enabled = @enabled, conditions = @conditions, updated_at = @now
WHERE id = @id AND user_id = @user_id;

-- name: SetWatchRuleEnabled :execrows
UPDATE watch_rules SET enabled = @enabled, updated_at = @now WHERE id = @id AND user_id = @user_id;

-- name: DeleteWatchRule :execrows
-- 作った視聴予約は、watch_rule_id を NULL にして残す (設計書 4.8)。
DELETE FROM watch_rules WHERE id = @id AND user_id = @user_id;

-- name: ListRuleWatchReservations :many
-- 視聴ルールが作った視聴予約のうち、番組の放送が始まっていないもの。照合で、一致しなくなったものを消すのに使う。
SELECT w.user_id, w.service_id, w.event_id, w.watch_rule_id
FROM watch_reservations w
JOIN programs p ON p.service_id = w.service_id AND p.event_id = w.event_id
WHERE w.watch_rule_id IS NOT NULL AND p.start_at > @now
ORDER BY w.user_id, w.service_id, w.event_id;

-- name: InsertRuleWatchReservations :execrows
-- 視聴ルールに一致した番組 (service_ids[i], event_ids[i]) に、rule_ids[i] の視聴ルールが作ったものとして、ユーザーの視聴予約を付ける。
-- 付けてある番組と、ユーザーが手で外した番組には付けない。
-- 通知を送る時刻を過ぎた番組にも付けない。通知のジョブは、通知した視聴予約を消すので、付け直すと二度通知してしまう。
INSERT INTO watch_reservations (user_id, service_id, event_id, created_at, watch_rule_id)
SELECT @user_id, p.service_id, p.event_id, @now, m.rule_id
FROM (
  SELECT unnest(@service_ids::bigint[]) AS service_id, unnest(@event_ids::bigint[]) AS event_id, unnest(@rule_ids::uuid[]) AS rule_id
) AS m
JOIN programs p ON p.service_id = m.service_id AND p.event_id = m.event_id
LEFT JOIN user_settings us ON us.user_id = @user_id
WHERE p.start_at - make_interval(mins => coalesce(us.watch_reminder_minutes, 5)) > @now
  AND NOT EXISTS (
    SELECT 1 FROM watch_rule_dismissals d
    WHERE d.user_id = @user_id AND d.service_id = p.service_id AND d.event_id = p.event_id
  )
ON CONFLICT (user_id, service_id, event_id) DO NOTHING;

-- name: DeleteRuleWatchReservations :execrows
-- ユーザーの視聴予約のうち、(service_ids[i], event_ids[i]) の組にあり、視聴ルールが作ったものを消す。手で付けたものは消さない。
DELETE FROM watch_reservations
WHERE user_id = @user_id AND watch_rule_id IS NOT NULL
  AND (service_id, event_id) IN (SELECT unnest(@service_ids::bigint[]), unnest(@event_ids::bigint[]));

-- name: InsertWatchRuleDismissal :exec
-- ユーザーが手で外した視聴予約の番組を覚える。視聴ルールに一致しても、付け直さない。
INSERT INTO watch_rule_dismissals (user_id, service_id, event_id, created_at)
VALUES (@user_id, @service_id, @event_id, @now)
ON CONFLICT (user_id, service_id, event_id) DO NOTHING;

-- name: DeleteWatchRuleDismissal :exec
DELETE FROM watch_rule_dismissals WHERE user_id = @user_id AND service_id = @service_id AND event_id = @event_id;

-- name: PruneWatchRuleDismissals :execrows
-- 番組が programs からなくなった (アーカイブに移ったか、EPG から消えた) 番組の、手で外した記録を消す。
DELETE FROM watch_rule_dismissals d
WHERE NOT EXISTS (SELECT 1 FROM programs p WHERE p.service_id = d.service_id AND p.event_id = d.event_id);
