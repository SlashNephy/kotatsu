-- name: AcquireEDCBNetworkTV :one
-- 範囲 [@start, @start + @count) のうち、空いている最も小さい ID を取る。取り合いになって空きがなくなれば、行を返さない。
INSERT INTO edcb_network_tvs (id, instance_id, network_id, service_id, acquired_at)
SELECT c.n, sqlc.arg(instance_id)::uuid, sqlc.arg(network_id)::integer, sqlc.arg(service_id)::integer, sqlc.arg(now)::timestamptz
FROM generate_series(sqlc.arg(start)::integer, sqlc.arg(start)::integer + sqlc.arg(count)::integer - 1) AS c(n)
WHERE NOT EXISTS (SELECT 1 FROM edcb_network_tvs t WHERE t.id = c.n)
ORDER BY c.n
LIMIT 1
ON CONFLICT (id) DO NOTHING
RETURNING id;

-- name: ReleaseEDCBNetworkTV :exec
DELETE FROM edcb_network_tvs WHERE id = @id AND instance_id = @instance_id;

-- name: ListOrphanedEDCBNetworkTVs :many
-- 持ち主のインスタンスが、どのロールでも生きていない ID を返す。取った直後の ID は、登録の前の持ち主のものかもしれないので含めない。
SELECT * FROM edcb_network_tvs t
WHERE t.acquired_at < @acquired_before
  AND NOT EXISTS (SELECT 1 FROM instances i WHERE i.id = t.instance_id AND i.heartbeat_at > @alive_since)
ORDER BY t.id;

-- name: DeleteEDCBNetworkTV :exec
DELETE FROM edcb_network_tvs WHERE id = @id AND instance_id = @instance_id;

-- name: CountEDCBNetworkTVs :one
SELECT count(*) FROM edcb_network_tvs WHERE id >= sqlc.arg(start)::integer AND id < sqlc.arg(start)::integer + sqlc.arg(count)::integer;
