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
