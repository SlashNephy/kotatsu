-- name: GetSchemaCompatibility :one
SELECT min_binary_version, schema_version FROM schema_compatibility WHERE id;

-- name: RecordSchemaVersion :exec
-- 下限とスキーマの版は上げるだけで、下げない。古いバイナリで migrate を実行しても、新しいバイナリが記録した値は残る。
INSERT INTO schema_compatibility (id, min_binary_version, schema_version, updated_at)
VALUES (true, @min_binary_version, @schema_version, @updated_at)
ON CONFLICT (id) DO UPDATE
SET
  min_binary_version = GREATEST(schema_compatibility.min_binary_version, EXCLUDED.min_binary_version),
  schema_version = GREATEST(schema_compatibility.schema_version, EXCLUDED.schema_version),
  updated_at = EXCLUDED.updated_at;

-- name: ListAppliedSchemaScripts :many
SELECT version FROM schema_scripts ORDER BY version;

-- name: MarkSchemaScriptsApplied :exec
INSERT INTO schema_scripts (version, applied_at)
SELECT unnest(@versions::bigint[]), @applied_at::timestamptz
ON CONFLICT (version) DO NOTHING;

-- name: ForgetSchemaScriptsAfter :exec
-- バックアップを戻した後に、書き出したときの版より新しいスクリプトを、戻した行に流し直すために消す。
DELETE FROM schema_scripts WHERE version > @version;

-- name: EnsureRecordingFailureCounts :exec
-- 失敗の理由ごとに 0 件の行を作っておき、一度も失敗していない理由も 0 として出す (設計書 5.5)。
INSERT INTO recording_failure_counts (reason)
SELECT unnest(@reasons::text[])
ON CONFLICT (reason) DO NOTHING;
