-- name: UpsertCaptionModel :exec
-- 画面からダウンロードしたモデルを、置き換えた後に記録する (設計書 6.1、#1682)。同じ種類の前の記録と置き換える。
INSERT INTO caption_models (kind, model_id, size, sha256, installed_at)
VALUES (@kind, @model_id, @size, @sha256, @now)
ON CONFLICT (kind) DO UPDATE SET model_id = EXCLUDED.model_id, size = EXCLUDED.size, sha256 = EXCLUDED.sha256,
  installed_at = EXCLUDED.installed_at;

-- name: ListCaptionModels :many
-- 画面からダウンロードして置いたモデルの記録を返す。
SELECT kind, model_id, size, sha256, installed_at FROM caption_models ORDER BY kind;

-- name: DeleteCaptionModel :execrows
-- 種類 kind のモデルの記録を消す。
DELETE FROM caption_models WHERE kind = @kind;
