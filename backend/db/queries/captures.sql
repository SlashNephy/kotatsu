-- name: InsertCapture :one
-- キャプチャを記録する (設計書 7.5)。録画が見つからなければ (撮った後に消されたなど)、録画の参照と再生位置を NULL にする。
INSERT INTO captures (
  id, user_id, recording_id, position_ms, service_name, program_name, service_name_normalized, program_name_normalized,
  caption, broadcast_at, taken_at, size, created_at
)
SELECT @id, @user_id, r.id, CASE WHEN r.id IS NULL THEN NULL ELSE sqlc.narg(position_ms)::bigint END,
  @service_name, sqlc.narg(program_name), @service_name_normalized, @program_name_normalized,
  sqlc.narg(caption), @broadcast_at, @taken_at, @size, @now
FROM (SELECT 1) AS one
LEFT JOIN recordings r ON r.id = sqlc.narg(recording_id)::uuid
RETURNING *;

-- name: GetCapture :one
-- user_id のユーザーのキャプチャ id を返す。ほかのユーザーのものは返さない。
SELECT * FROM captures WHERE id = @id AND user_id = @user_id;

-- name: ListCaptures :many
-- user_id のユーザーのキャプチャを、記録した新しい順に返す。
-- words は、NFKC で正規化して LIKE のために逃がした語。すべての語が、正規化した番組名かサービス名に含まれるものだけを返す。
-- before_at と before_id より前 (古い) のキャプチャだけを返し、ページを送る。2 つは、両方を渡すか、両方を NULL にする。
SELECT * FROM captures c
WHERE c.user_id = @user_id
  AND NOT EXISTS (
    SELECT 1 FROM unnest(@words::text[]) AS w (word)
    WHERE NOT (c.program_name_normalized ILIKE '%' || w.word || '%' OR c.service_name_normalized ILIKE '%' || w.word || '%')
  )
  AND (sqlc.narg(before_at)::timestamptz IS NULL
    OR (c.created_at, c.id) < (sqlc.narg(before_at)::timestamptz, sqlc.narg(before_id)::uuid))
ORDER BY c.created_at DESC, c.id DESC
LIMIT @max_count;

-- name: DeleteCapture :one
-- user_id のユーザーのキャプチャ id を消し、消した行を返す。ほかのユーザーのものは消さない。
DELETE FROM captures WHERE id = @id AND user_id = @user_id RETURNING *;

-- name: ListCaptureLocations :many
-- すべてのキャプチャの持ち主と ID。データディレクトリの、DB にないファイルを探すときに、画像を使っているものとみなすのに使う。
SELECT id, user_id FROM captures;
