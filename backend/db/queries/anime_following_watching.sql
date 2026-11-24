-- name: DeleteAnimeFollowingWatching :exec
-- ユーザーの、記録先のフォローしている人が見ている作品を消す (#1968)。取り込み直すたびに入れ替える。
DELETE FROM anime_following_watching WHERE user_id = @user_id AND destination = @destination;

-- name: InsertAnimeFollowingWatching :exec
-- フォローしている人が見ている作品を、まとめて書く (#1968)。同じ人の同じ作品が 2 度返ってきたら、先のものを使う。
-- 配列は NULL を持てないので、アバターの URL が分からなければ空で渡す。
INSERT INTO anime_following_watching (user_id, destination, username, name, avatar_url, work_id, title, fetched_at)
SELECT @user_id, @destination, t.username, t.name, NULLIF(t.avatar_url, ''), t.work_id, t.title, @now
FROM (
  SELECT unnest(@usernames::text[]) AS username, unnest(@names::text[]) AS name, unnest(@avatar_urls::text[]) AS avatar_url,
    unnest(@work_ids::integer[]) AS work_id, unnest(@titles::text[]) AS title
) AS t
ON CONFLICT (user_id, destination, username, work_id) DO NOTHING;

-- name: ListAnimeFollowingWatching :many
-- ユーザーの、フォローしている人が見ている作品を返す (#1968)。fetched_after より前に取り込んだものは返さない。
-- 作品ごとの視聴状況 (#1372) と同じく、利用者の情報を、表示に要る範囲を超えて蓄積しないため。
SELECT destination, username, name, avatar_url, work_id, title
FROM anime_following_watching
WHERE user_id = @user_id AND fetched_at >= @fetched_after::timestamptz
ORDER BY destination, work_id, username;

-- name: PruneAnimeFollowingWatching :execrows
-- before より前に取り込んだ、フォローしている人が見ている作品を消す (#1968)。取り込み直せなかった連携の古い分を残さない。
DELETE FROM anime_following_watching WHERE fetched_at < @before::timestamptz;
