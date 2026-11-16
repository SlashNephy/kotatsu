-- name: LockEDCBNetworkTVService :exec
-- 同じサービスの ID を取るのと加わるのを、インスタンスをまたいで 1 つずつにする (#1850)。トランザクションの終わりまで持つ。
-- 2 つのインスタンスが同時に同じサービスを開いても、別々の ID を取って、チューナーを 2 本使わないようにする。
SELECT pg_advisory_xact_lock(hashtext('kotatsu.edcb_network_tv'), hashtext(sqlc.arg(network_id)::integer || '/' || sqlc.arg(service_id)::integer));

-- name: FindJoinableEDCBNetworkTV :one
-- 範囲 [@start, @start + @count) のうち、同じサービスを開いていて、閉じ始めておらず、生きている持ち主が @max_holders に満たない ID を返す (#1850)。
-- 生きている持ち主は、インスタンスが AliveTTL のうちにハートビートを書いたか、取った直後のもの。止まった持ち主の中継は切れているので、数えない。
-- 行を FOR UPDATE で押さえ、加わるまでに、ほかのインスタンスが閉じ始めないようにする。
SELECT t.id FROM edcb_network_tvs t
WHERE t.network_id = sqlc.arg(network_id)::integer AND t.service_id = sqlc.arg(service_id)::integer
  AND t.id >= sqlc.arg(start)::integer AND t.id < sqlc.arg(start)::integer + sqlc.arg(count)::integer
  AND t.closing_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM edcb_network_tv_holders h WHERE h.network_tv_id = t.id AND h.instance_id = sqlc.arg(instance_id)::uuid)
  AND (
    SELECT count(*) FROM edcb_network_tv_holders h
    WHERE h.network_tv_id = t.id
      AND (h.acquired_at > sqlc.arg(alive_since)::timestamptz
        OR EXISTS (SELECT 1 FROM instances i WHERE i.id = h.instance_id AND i.heartbeat_at > sqlc.arg(alive_since)::timestamptz))
  ) < sqlc.arg(max_holders)::integer
ORDER BY t.id
LIMIT 1
FOR UPDATE OF t;

-- name: AcquireEDCBNetworkTV :one
-- 範囲 [@start, @start + @count) のうち、空いている最も小さい ID を取る。取り合いになって空きがなくなれば、行を返さない。
INSERT INTO edcb_network_tvs (id, network_id, service_id, acquired_at)
SELECT c.n, sqlc.arg(network_id)::integer, sqlc.arg(service_id)::integer, sqlc.arg(now)::timestamptz
FROM generate_series(sqlc.arg(start)::integer, sqlc.arg(start)::integer + sqlc.arg(count)::integer - 1) AS c(n)
WHERE NOT EXISTS (SELECT 1 FROM edcb_network_tvs t WHERE t.id = c.n)
ORDER BY c.n
LIMIT 1
ON CONFLICT (id) DO NOTHING
RETURNING id;

-- name: AddEDCBNetworkTVHolder :exec
-- @instance_id を ID の持ち主に加え、最後の持ち主が外れた印を消す。
WITH holder AS (
  INSERT INTO edcb_network_tv_holders (network_tv_id, instance_id, acquired_at)
  VALUES (@network_tv_id, @instance_id, @now)
)
UPDATE edcb_network_tvs SET released_at = NULL WHERE id = @network_tv_id;

-- name: LockEDCBNetworkTV :one
-- ID の行を押さえる。加わるのと、外れて閉じ始めるのを、1 つずつにする。行がなければ返さない。
SELECT * FROM edcb_network_tvs WHERE id = @id FOR UPDATE;

-- name: DeleteEDCBNetworkTVHolder :exec
DELETE FROM edcb_network_tv_holders WHERE network_tv_id = @network_tv_id AND instance_id = @instance_id;

-- name: CountAliveEDCBNetworkTVHolders :one
-- ID の、生きている持ち主の数を返す。生きているかは FindJoinableEDCBNetworkTV と同じ。
SELECT count(*) FROM edcb_network_tv_holders h
WHERE h.network_tv_id = @network_tv_id
  AND (h.acquired_at > sqlc.arg(alive_since)::timestamptz
    OR EXISTS (SELECT 1 FROM instances i WHERE i.id = h.instance_id AND i.heartbeat_at > sqlc.arg(alive_since)::timestamptz));

-- name: MarkEDCBNetworkTVReleased :exec
UPDATE edcb_network_tvs SET released_at = sqlc.arg(now)::timestamptz WHERE id = @id;

-- name: MarkEDCBNetworkTVClosing :exec
UPDATE edcb_network_tvs SET closing_at = sqlc.arg(now)::timestamptz WHERE id = @id;

-- name: DeleteEDCBNetworkTV :exec
DELETE FROM edcb_network_tvs WHERE id = @id;

-- name: ListOrphanedEDCBNetworkTVs :many
-- 生きている持ち主のいない ID を返す。閉じ始めた ID は、閉じ始めてから @closing_before を過ぎたものだけ返す
-- (閉じる途中で止まったインスタンスの ID)。取った直後と、最後の持ち主が外れた直後の ID は、
-- 登録の前の持ち主のものか、ほかのインスタンスが引き継ぐかもしれないので含めない。
SELECT t.* FROM edcb_network_tvs t
WHERE t.acquired_at < sqlc.arg(alive_since)::timestamptz
  AND (t.released_at IS NULL OR t.released_at < sqlc.arg(alive_since)::timestamptz)
  AND (t.closing_at IS NULL OR t.closing_at < sqlc.arg(closing_before)::timestamptz)
  AND NOT EXISTS (
    SELECT 1 FROM edcb_network_tv_holders h
    WHERE h.network_tv_id = t.id
      AND (h.acquired_at > sqlc.arg(alive_since)::timestamptz
        OR EXISTS (SELECT 1 FROM instances i WHERE i.id = h.instance_id AND i.heartbeat_at > sqlc.arg(alive_since)::timestamptz))
  )
ORDER BY t.id;

-- name: DeleteDeadEDCBNetworkTVHolders :execrows
-- 止まったインスタンスの持ち主の行を消す。ID は、ほかの持ち主が使い続けるので残す。持ち主がいなくなった ID は、ListOrphanedEDCBNetworkTVs で閉じる。
DELETE FROM edcb_network_tv_holders h
WHERE h.acquired_at < sqlc.arg(alive_since)::timestamptz
  AND NOT EXISTS (SELECT 1 FROM instances i WHERE i.id = h.instance_id AND i.heartbeat_at > sqlc.arg(alive_since)::timestamptz);

-- name: CountEDCBNetworkTVs :one
SELECT count(*) FROM edcb_network_tvs WHERE id >= sqlc.arg(start)::integer AND id < sqlc.arg(start)::integer + sqlc.arg(count)::integer;
