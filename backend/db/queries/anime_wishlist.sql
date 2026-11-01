-- name: ListDueAnimeWishlistConnections :many
-- 見たいリストを取り込み直す時期が来た連携 (#928) を返す。一度も問い合わせていない連携と、前に取り込めてから
-- succeeded_before より前に問い合わせた連携と、取り込めなかったときは、問い合わせ直す日時が now を過ぎた連携。
SELECT user_id, destination FROM anime_record_connections
WHERE wishlist_checked_at IS NULL
  OR (wishlist_error IS NULL AND wishlist_checked_at <= @succeeded_before::timestamptz)
  OR (wishlist_error IS NOT NULL AND COALESCE(wishlist_retry_at, wishlist_checked_at) <= @now::timestamptz)
ORDER BY wishlist_checked_at NULLS FIRST, user_id, destination;

-- name: DeleteAnimeWishlistEntries :exec
DELETE FROM anime_wishlist_entries WHERE user_id = @user_id AND destination = @destination;

-- name: InsertAnimeWishlistEntries :exec
-- 同じ作品が 2 度返ってきたら (見たいと見てるの両方など)、先に返ってきたものを使う。
INSERT INTO anime_wishlist_entries (user_id, destination, work_id, status, title, fetched_at)
SELECT @user_id, @destination, t.work_id, t.status, t.title, @now
FROM (
  SELECT unnest(@work_ids::integer[]) AS work_id, unnest(@statuses::text[]) AS status, unnest(@titles::text[]) AS title
) AS t
ON CONFLICT (user_id, destination, work_id) DO NOTHING;

-- name: FinishAnimeWishlistImport :execrows
-- 見たいリストを問い合わせた結果を残す。取り込めたら (error が NULL)、取り込めた日時を進め、理由を消す。
-- 取り込めなければ、問い合わせ直す日時 (retry_at) を残す。
-- 連携を外した後なら、何も書かない。
UPDATE anime_record_connections SET
  wishlist_checked_at = @now::timestamptz,
  wishlist_imported_at = CASE WHEN sqlc.narg('error')::text IS NULL THEN @now::timestamptz ELSE wishlist_imported_at END,
  wishlist_error = sqlc.narg('error')::text,
  wishlist_retry_at = sqlc.narg('retry_at')::timestamptz
WHERE user_id = @user_id AND destination = @destination;

-- name: ListAnimeWishlistEntries :many
SELECT * FROM anime_wishlist_entries WHERE user_id = @user_id ORDER BY destination, work_id;

-- name: ListAnimeWorksByExternalIDs :many
-- Annict の作品 ID か AniList の ID が、どれかに当たる作品の対応を返す (#928)。
SELECT w.syobocal_tid, w.annict_work_id, w.anilist_id
FROM anime_works w
WHERE (w.annict_work_id = ANY(@annict_work_ids::integer[]) OR w.anilist_id = ANY(@anilist_ids::integer[]))
ORDER BY w.syobocal_tid, w.position;

-- name: ListAnimeWishlistPrograms :many
-- TID に結びついた番組のうち、since から until までに始まる、終了時刻の決まった番組を返す (#928)。
-- 2 話連続の放送のように、1 つの番組が同じ TID の話に何度も結びついても、1 行にする。
SELECT DISTINCT ON (p.service_id, p.event_id, e.syobocal_tid)
  e.syobocal_tid::integer AS syobocal_tid, p.service_id, p.event_id, p.start_at, p.end_at, p.name, p.related_items
FROM program_anime_episodes e
JOIN program_anime_matches m ON m.service_id = e.service_id AND m.start_at = e.start_at
JOIN programs p ON p.service_id = m.service_id AND p.event_id = m.event_id AND p.start_at = m.start_at
WHERE e.syobocal_tid = ANY(@syobocal_tids::integer[])
  AND p.start_at >= @since AND p.start_at < @until AND NOT p.end_undecided
ORDER BY p.service_id, p.event_id, e.syobocal_tid;

-- name: LockAnimeWishlistRule :exec
-- 見たいリストの作品からルールを作るとき (#928)、同じ作品のルールを重ねて作らないよう、TID ごとにトランザクションの終わりまで待ち合わせる。
SELECT pg_advisory_xact_lock(hashtext('kotatsu.anime_wishlist_rule'), @syobocal_tid::integer);
