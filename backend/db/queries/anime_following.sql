-- name: ListAnimeFollowingFetches :many
-- ユーザーが連携した記録先ごとに、作品 (TID) のフォロー中のユーザーの視聴状況を問い合わせた様子を返す (#1372)。
-- まだ問い合わせていなければ、checked_at などは NULL。
SELECT c.destination, f.checked_at, f.fetched_at, f.error, f.retry_at
FROM anime_record_connections c
LEFT JOIN anime_following_fetches f
  ON f.user_id = c.user_id AND f.destination = c.destination AND f.syobocal_tid = @syobocal_tid
WHERE c.user_id = @user_id
ORDER BY c.destination;

-- name: ListAnimeFollowingViewings :many
-- ユーザーの、作品 (TID) のフォロー中のユーザーの視聴状況を返す。保持する期間を過ぎたものは、消す前でも返さない。
SELECT v.*
FROM anime_following_viewings v
JOIN anime_following_fetches f USING (user_id, destination, syobocal_tid)
WHERE v.user_id = @user_id AND v.syobocal_tid = @syobocal_tid AND f.fetched_at >= @fetched_after::timestamptz
ORDER BY v.destination, v.username;

-- name: SaveAnimeFollowingFetch :execrows
-- 問い合わせた結果を残す。取り込めたら (error が NULL)、取り込めた日時を進める。取り込めなければ、前に取り込めた日時を残す。
-- 連携を外した後なら、何も書かない。
INSERT INTO anime_following_fetches (user_id, destination, syobocal_tid, checked_at, fetched_at, error, retry_at)
SELECT c.user_id, c.destination, @syobocal_tid::integer, @now::timestamptz,
  CASE WHEN sqlc.narg('error')::text IS NULL THEN @now::timestamptz END,
  sqlc.narg('error')::text, sqlc.narg('retry_at')::timestamptz
FROM anime_record_connections c
WHERE c.user_id = @user_id AND c.destination = @destination
ON CONFLICT (user_id, destination, syobocal_tid) DO UPDATE SET
  checked_at = EXCLUDED.checked_at,
  fetched_at = COALESCE(EXCLUDED.fetched_at, anime_following_fetches.fetched_at),
  error = EXCLUDED.error,
  retry_at = EXCLUDED.retry_at;

-- name: DeleteAnimeFollowingViewings :exec
DELETE FROM anime_following_viewings
WHERE user_id = @user_id AND destination = @destination AND syobocal_tid = @syobocal_tid;

-- name: InsertAnimeFollowingViewings :exec
-- 配列は NULL を持てないので、分からない数は -1 で、分からない文字列は空で渡す。
INSERT INTO anime_following_viewings (
  user_id, destination, syobocal_tid, username, name, avatar_url, status, progress, episodes_count, score, notes
)
SELECT @user_id, @destination, @syobocal_tid, t.username, t.name, NULLIF(t.avatar_url, ''), t.status,
  NULLIF(t.progress, -1), NULLIF(t.episodes_count, -1), NULLIF(t.score, -1), NULLIF(t.notes, '')
FROM (
  SELECT unnest(@usernames::text[]) AS username, unnest(@names::text[]) AS name, unnest(@avatar_urls::text[]) AS avatar_url,
    unnest(@statuses::text[]) AS status, unnest(@progresses::integer[]) AS progress, unnest(@episodes_counts::integer[]) AS episodes_count,
    unnest(@scores::double precision[]) AS score, unnest(@notes::text[]) AS notes
) AS t;

-- name: PruneAnimeFollowingFetches :execrows
-- 最後に取り込めてから (一度も取り込めていなければ、問い合わせてから) before より前のものを消す。視聴状況も一緒に消える。
DELETE FROM anime_following_fetches WHERE COALESCE(fetched_at, checked_at) < @before::timestamptz;

-- name: GetAnimeFollowingAvatarURL :one
-- ユーザーが取り込んだ、フォロー中のユーザーのアバターの URL を返す。保持する期間を過ぎたものは返さない。
-- 同じ人が複数の作品に出るので、最も新しく取り込んだ URL を使う。
SELECT v.avatar_url FROM anime_following_viewings v
JOIN anime_following_fetches f USING (user_id, destination, syobocal_tid)
WHERE v.user_id = @user_id AND v.destination = @destination AND v.username = @username AND v.avatar_url IS NOT NULL
  AND f.fetched_at >= @fetched_after::timestamptz
ORDER BY f.fetched_at DESC
LIMIT 1;
