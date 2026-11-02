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
-- 配列は NULL を持てないので、分からない数は -1 で、分からない文字列は空で渡す。
INSERT INTO anime_wishlist_entries (
  user_id, destination, work_id, status, title, next_episode_id, next_episode_number, next_episode_number_text, next_episode_title,
  progress, episodes_count, season_year, season_name, image_url, fetched_at
)
SELECT @user_id, @destination, t.work_id, t.status, t.title, NULLIF(t.next_episode_id, ''), NULLIF(t.next_episode_number, -1),
  NULLIF(t.next_episode_number_text, ''), NULLIF(t.next_episode_title, ''), NULLIF(t.progress, -1), NULLIF(t.episodes_count, -1),
  NULLIF(t.season_year, -1), NULLIF(t.season_name, ''), NULLIF(t.image_url, ''), @now
FROM (
  SELECT unnest(@work_ids::integer[]) AS work_id, unnest(@statuses::text[]) AS status, unnest(@titles::text[]) AS title,
    unnest(@next_episode_ids::text[]) AS next_episode_id, unnest(@next_episode_numbers::integer[]) AS next_episode_number,
    unnest(@next_episode_number_texts::text[]) AS next_episode_number_text, unnest(@next_episode_titles::text[]) AS next_episode_title,
    unnest(@progresses::integer[]) AS progress, unnest(@episodes_counts::integer[]) AS episodes_count,
    unnest(@season_years::integer[]) AS season_year, unnest(@season_names::text[]) AS season_name, unnest(@image_urls::text[]) AS image_url
) AS t
ON CONFLICT (user_id, destination, work_id) DO NOTHING;

-- name: AdvanceAnimeWishlistProgress :exec
-- kotatsu が記録先に話を記録できたら、次に取り込むまでの間も画面が古くならないよう、見たいリストの進み具合を進める (#1230)。
-- 進み具合は戻さない。記録した話数以下の「次に見る話」は、もう次ではないので消し、次に取り込むときに引き直す。
UPDATE anime_wishlist_entries SET
  progress = GREATEST(COALESCE(progress, 0), @episode_number::integer),
  next_episode_id = CASE WHEN next_episode_number <= @episode_number::integer THEN NULL ELSE next_episode_id END,
  next_episode_number_text = CASE WHEN next_episode_number <= @episode_number::integer THEN NULL ELSE next_episode_number_text END,
  next_episode_title = CASE WHEN next_episode_number <= @episode_number::integer THEN NULL ELSE next_episode_title END,
  next_episode_number = CASE WHEN next_episode_number <= @episode_number::integer THEN NULL ELSE next_episode_number END
WHERE user_id = @user_id AND destination = @destination AND work_id = ANY(@work_ids::integer[]);

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

-- name: ListAnimeLibraryPrograms :many
-- 視聴管理の画面 (#1231) の、TID に結びついた番組のうち、until より前に始まり、now より後に終わる番組を返す。放送中の番組も次の放送とする。
-- 2 話連続の放送のように、1 つの番組が同じ TID の話に何度も結びついたら、話数の範囲をまとめて 1 行にする。
-- 話数のない放送 (特番など) の話数は、-1 にする。
SELECT e.syobocal_tid::integer AS syobocal_tid, p.service_id, p.event_id, p.start_at,
  COALESCE(min(e.count_start), -1)::double precision AS count_start, COALESCE(max(e.count_end), -1)::double precision AS count_end
FROM program_anime_episodes e
JOIN program_anime_matches m ON m.service_id = e.service_id AND m.start_at = e.start_at
JOIN programs p ON p.service_id = m.service_id AND p.event_id = m.event_id AND p.start_at = m.start_at
WHERE e.syobocal_tid = ANY(@syobocal_tids::integer[]) AND p.end_at > @now AND p.start_at < @until
GROUP BY e.syobocal_tid, p.service_id, p.event_id, p.start_at
ORDER BY p.start_at, p.service_id, e.syobocal_tid;

-- name: ListAnimeLibraryRecordings :many
-- 視聴管理の画面 (#1231) の、TID に結びついた話の録画を返す。話数のない録画と、録れなかった録画は返さない。
-- 同じ話の録画は、新しく録ったものを先に並べる。
SELECT e.syobocal_tid::integer AS syobocal_tid, e.count_start::double precision AS count_start,
  e.count_end::double precision AS count_end, e.subtitle, r.id AS recording_id
FROM recording_anime_episodes e
JOIN recordings r ON r.id = e.recording_id
WHERE e.syobocal_tid = ANY(@syobocal_tids::integer[]) AND e.count_start IS NOT NULL AND e.count_end IS NOT NULL AND r.state <> 'failed'
ORDER BY e.syobocal_tid, e.count_start, e.count_end, r.recorded_from DESC, r.id;
