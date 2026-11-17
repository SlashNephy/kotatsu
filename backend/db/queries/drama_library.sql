-- name: ListDramaWatchlistEntries :many
-- ユーザーの見ている作品と見たい作品 (#1652) を、足した順に返す。
SELECT * FROM drama_watchlist_entries WHERE user_id = @user_id ORDER BY created_at, tmdb_series_id;

-- name: GetDramaWatchlistStatus :one
SELECT status FROM drama_watchlist_entries WHERE user_id = @user_id AND tmdb_series_id = @tmdb_series_id;

-- name: UpsertDramaWatchlistEntry :exec
-- 作品を見ている作品か見たい作品に足す。足してあれば、状態だけを変え、足した日時は残す。
INSERT INTO drama_watchlist_entries (user_id, tmdb_series_id, status, created_at, updated_at)
VALUES (@user_id, @tmdb_series_id, @status, @now, @now)
ON CONFLICT (user_id, tmdb_series_id) DO UPDATE SET status = excluded.status, updated_at = excluded.updated_at;

-- name: DeleteDramaWatchlistEntry :execrows
DELETE FROM drama_watchlist_entries WHERE user_id = @user_id AND tmdb_series_id = @tmdb_series_id;

-- name: DramaSeriesLinked :one
-- 作品が、番組か録画に結びついているかどうか。見ている作品には、結びつきのある作品だけを足せる。
SELECT (EXISTS (SELECT 1 FROM program_drama_matches p WHERE p.tmdb_series_id = @series_id::integer)
  OR EXISTS (SELECT 1 FROM recording_drama_matches r WHERE r.tmdb_series_id = @series_id::integer))::boolean AS linked;

-- name: InsertDramaWatchedEpisode :execrows
-- 見た話を足す。もう見た話なら、何もしない (見た日時と出どころは、最初に足したものを残す)。
INSERT INTO drama_watched_episodes (user_id, tmdb_series_id, season_number, episode_number, source, watched_at)
VALUES (@user_id, @tmdb_series_id, @season_number, @episode_number, @source, @now)
ON CONFLICT (user_id, tmdb_series_id, season_number, episode_number) DO NOTHING;

-- name: DeleteDramaWatchedEpisode :execrows
DELETE FROM drama_watched_episodes
WHERE user_id = @user_id AND tmdb_series_id = @tmdb_series_id AND season_number = @season_number AND episode_number = @episode_number;

-- name: DramaEpisodeWatched :one
SELECT EXISTS (
  SELECT 1 FROM drama_watched_episodes
  WHERE user_id = @user_id AND tmdb_series_id = @tmdb_series_id AND season_number = @season_number AND episode_number = @episode_number
) AS watched;

-- name: ListDramaWatchedEpisodes :many
-- ユーザーが見た、作品 tmdb_series_ids の話を返す。
SELECT tmdb_series_id, season_number, episode_number FROM drama_watched_episodes
WHERE user_id = @user_id AND tmdb_series_id = ANY(@tmdb_series_ids::integer[])
ORDER BY tmdb_series_id, season_number, episode_number;

-- name: ListDramaLibraryPrograms :many
-- 視聴管理の画面 (#1652) の、作品に結びついた番組のうち、until より前に始まり、now より後に終わる番組を返す。放送中の番組も次の放送とする。
SELECT m.tmdb_series_id::integer AS tmdb_series_id, m.season_number, m.episode_number,
  p.service_id, p.event_id, p.start_at, p.name, p.series_repeat
FROM program_drama_matches m
JOIN programs p ON p.service_id = m.service_id AND p.event_id = m.event_id AND p.start_at = m.start_at
WHERE m.tmdb_series_id = ANY(@tmdb_series_ids::integer[]) AND p.end_at > @now AND p.start_at < @until
ORDER BY p.start_at, p.service_id;

-- name: ListDramaLibraryRecentBroadcasts :many
-- 視聴管理の画面 (#1652) の、作品ごとの、since から now までに始まった最も新しい放送を返す。「昨日」「終了」のバッジに使う。
-- 過去の番組は、放送が終わってしばらくするとアーカイブに移るので、番組ではなく、番組の結びつきの開始時刻と話だけを読む。
SELECT DISTINCT ON (m.tmdb_series_id)
  m.tmdb_series_id::integer AS tmdb_series_id, m.start_at, m.season_number, m.episode_number
FROM program_drama_matches m
WHERE m.tmdb_series_id = ANY(@tmdb_series_ids::integer[]) AND m.start_at >= @since AND m.start_at < @now
ORDER BY m.tmdb_series_id, m.start_at DESC, m.service_id;

-- name: ListDramaLibraryRecordings :many
-- 視聴管理の画面 (#1652) の、作品の話に結びついた録画を返す。話のない録画と、録れなかった録画は返さない。
-- 同じ話の録画は、新しく録ったものを先に並べる。
SELECT m.tmdb_series_id::integer AS tmdb_series_id, m.season_number::integer AS season_number,
  m.episode_number::integer AS episode_number, r.id AS recording_id
FROM recording_drama_matches m
JOIN recordings r ON r.id = m.recording_id
WHERE m.tmdb_series_id = ANY(@tmdb_series_ids::integer[]) AND m.episode_number IS NOT NULL AND r.state <> 'failed'
ORDER BY m.tmdb_series_id, m.season_number, m.episode_number, r.recorded_from DESC, r.id;

-- name: ListTMDbSeasonsOf :many
-- 作品 series_ids の、キャッシュにあるシーズンを返す。
SELECT * FROM tmdb_seasons WHERE series_id = ANY(@series_ids::integer[]) ORDER BY series_id, season_number;
