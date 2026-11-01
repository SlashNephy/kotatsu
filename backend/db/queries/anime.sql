-- name: GetAnimeImport :one
SELECT * FROM anime_imports WHERE source = @source;

-- name: ListAnimeImports :many
SELECT * FROM anime_imports ORDER BY source;

-- name: RecordAnimeImport :exec
-- 取り込めたことを記録する。内容が変わっていなければ (304)、呼ぶ側が前の ETag と件数を渡す。
INSERT INTO anime_imports (source, etag, checked_at, imported_at, item_count, error)
VALUES (@source, sqlc.narg(etag), @now, @now, @item_count, NULL)
ON CONFLICT (source) DO UPDATE SET
  etag = excluded.etag, checked_at = excluded.checked_at, imported_at = excluded.imported_at,
  item_count = excluded.item_count, error = NULL;

-- name: RecordAnimeImportFailure :exec
-- 取り込めなかった理由を記録する。前に取り込めた内容と ETag は残し、1 時間後にまた問い合わせる。
INSERT INTO anime_imports (source, etag, checked_at, imported_at, item_count, error)
VALUES (@source, NULL, @now, NULL, 0, @error)
ON CONFLICT (source) DO UPDATE SET checked_at = excluded.checked_at, error = excluded.error;

-- name: LockAnimeImport :exec
-- 取り込み元 source の入れ替えを、1 つずつ行わせる。トランザクションが終わるまでロックする。
-- 取り込みのジョブが重なると、後の入れ替えが、先に入れた行と主キーでぶつかり、取り込めなかったと記録してしまうため。
SELECT pg_advisory_xact_lock(hashtext('kotatsu.anime_import'), hashtext(@source::text));

-- name: DeleteAnimeChannelDefinitions :exec
DELETE FROM anime_channel_definitions;

-- name: InsertAnimeChannelDefinitions :exec
-- 同じサービスは 1 度だけ渡す (先に出てきたものを使うのは、呼ぶ側で決める)。配列は NULL を持てないので、対応のない ID は 0 で渡す。
INSERT INTO anime_channel_definitions (channel_type, network_id, sid, name, syobocal_channel_id, annict_channel_id)
SELECT t.channel_type, t.network_id, t.sid, t.name, NULLIF(t.syobocal_channel_id, 0), NULLIF(t.annict_channel_id, 0)
FROM (
  SELECT unnest(@channel_types::text[]) AS channel_type, unnest(@network_ids::bigint[]) AS network_id,
    unnest(@sids::bigint[]) AS sid, unnest(@names::text[]) AS name,
    unnest(@syobocal_channel_ids::integer[]) AS syobocal_channel_id, unnest(@annict_channel_ids::integer[]) AS annict_channel_id
) AS t;

-- name: DeleteAnimeWorks :exec
DELETE FROM anime_works WHERE source = @source;

-- name: InsertAnimeWorks :exec
-- 配列は NULL を持てないので、対応のない ID は 0 で渡す。
INSERT INTO anime_works (source, syobocal_tid, position, annict_work_id, anilist_id, mal_id)
SELECT @source, t.syobocal_tid, t.position, NULLIF(t.annict_work_id, 0), NULLIF(t.anilist_id, 0), NULLIF(t.mal_id, 0)
FROM (
  SELECT unnest(@syobocal_tids::integer[]) AS syobocal_tid, unnest(@positions::smallint[]) AS position,
    unnest(@annict_work_ids::integer[]) AS annict_work_id, unnest(@anilist_ids::integer[]) AS anilist_id,
    unnest(@mal_ids::integer[]) AS mal_id
) AS t;

-- name: ListAnimeWorks :many
-- TID ごとに、arm-supplementary の行があればそちらを、なければ arm の行を返す (設計書 3.8)。
SELECT w.syobocal_tid, w.position, w.annict_work_id, w.anilist_id, w.mal_id
FROM anime_works w
WHERE w.syobocal_tid = ANY(@syobocal_tids::integer[])
  AND (w.source = 'arm_supplementary' OR NOT EXISTS (
    SELECT 1 FROM anime_works s WHERE s.source = 'arm_supplementary' AND s.syobocal_tid = w.syobocal_tid
  ))
ORDER BY w.syobocal_tid, w.position;

-- name: ListAnimeChannels :many
-- サービスごとに、取り込んだ対応と管理者の上書き、使う値を返す (設計書 3.8)。
-- 地上波は種別とサービス ID で、BS と CS はネットワーク ID とサービス ID で、取り込んだ対応と照らし合わせる。
SELECT s.id AS service_id,
  d.name AS definition_name,
  d.syobocal_channel_id AS imported_syobocal_channel_id,
  d.annict_channel_id AS imported_annict_channel_id,
  o.syobocal_channel_id AS override_syobocal_channel_id,
  o.annict_channel_id AS override_annict_channel_id,
  COALESCE(o.syobocal_channel_id, d.syobocal_channel_id) AS syobocal_channel_id,
  COALESCE(o.annict_channel_id, d.annict_channel_id) AS annict_channel_id
FROM services s
LEFT JOIN anime_channel_definitions d ON d.sid = s.sid AND d.channel_type = s.channel_type
  AND (d.network_id = s.network_id OR (d.channel_type = 'GR' AND d.network_id = 0))
LEFT JOIN anime_channel_overrides o ON o.service_id = s.id
ORDER BY s.id;

-- name: LockAnimeChannelOverride :one
-- 変える前の値を監査ログに残すため、上書きの行をロックして読む。サービスがなければ行を返さない。
SELECT s.id AS service_id, o.syobocal_channel_id, o.annict_channel_id
FROM services s LEFT JOIN anime_channel_overrides o ON o.service_id = s.id
WHERE s.id = @service_id
FOR UPDATE OF s;

-- name: UpsertAnimeChannelOverride :exec
INSERT INTO anime_channel_overrides (service_id, syobocal_channel_id, annict_channel_id, updated_at)
VALUES (@service_id, sqlc.narg(syobocal_channel_id), sqlc.narg(annict_channel_id), @now)
ON CONFLICT (service_id) DO UPDATE SET
  syobocal_channel_id = excluded.syobocal_channel_id, annict_channel_id = excluded.annict_channel_id, updated_at = excluded.updated_at;

-- name: DeleteAnimeChannelOverride :exec
DELETE FROM anime_channel_overrides WHERE service_id = @service_id;
