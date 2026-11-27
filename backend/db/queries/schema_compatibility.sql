-- name: GetMinBinaryVersion :one
SELECT min_binary_version FROM schema_compatibility WHERE id;

-- name: RaiseMinBinaryVersion :exec
-- 下限は上げるだけで、下げない。古いバイナリで migrate を実行しても、新しいバイナリが記録した下限は残る。
INSERT INTO schema_compatibility (id, min_binary_version, updated_at)
VALUES (true, @min_binary_version, @updated_at)
ON CONFLICT (id) DO UPDATE
SET
  min_binary_version = GREATEST(schema_compatibility.min_binary_version, EXCLUDED.min_binary_version),
  updated_at = EXCLUDED.updated_at;

-- name: GetAcquireCompatibility :one
-- 動かしてよいバイナリの下限と、下限を満たすバイナリで動いている、holder のほかの role のインスタンスの数を返す (#2195)。
-- 下限より古い mikan は、下限を満たす mikan がほかにいれば、割り振りを取らずに任せる。
SELECT c.min_binary_version, (
  SELECT count(*) FROM instances i
  WHERE i.role = @role AND i.heartbeat_at > @alive_since AND i.id <> @holder AND i.schema_version >= c.min_binary_version
    -- 割り振りを取らずにいる mikan は、任せる先に数えない (#2200)。
    AND (i.acquire_paused_until IS NULL OR i.acquire_paused_until <= @now::timestamptz)
)::bigint AS compatible_peers
FROM schema_compatibility c WHERE c.id;
