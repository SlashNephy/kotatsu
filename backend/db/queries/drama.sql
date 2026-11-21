-- name: GetDramaMatchCursor :one
SELECT epg_version FROM drama_match_cursor WHERE id = 1;

-- name: SaveDramaMatchCursor :exec
INSERT INTO drama_match_cursor (id, epg_version, updated_at) VALUES (1, @epg_version, @now)
ON CONFLICT (id) DO UPDATE SET epg_version = excluded.epg_version, updated_at = excluded.updated_at;

-- name: ListDramaPrograms :many
-- サービス service_ids の、from から to までと放送時間が重なる、ドラマ (最初のジャンルの大分類が 0x3) の番組を返す。
-- ドラマのジャンルを 2 つ目以降に持つ番組 (情報番組やバラエティで、ドラマの宣伝をするものなど) は照合しない。
-- service_ids が NULL なら、すべてのサービスの番組を返す。問い合わせの上限で区切っても、近い放送から照合するよう、開始時刻の順に並べる。
SELECT p.service_id, p.event_id, p.start_at, p.end_at, p.end_undecided, p.name, p.series_episode, p.series_repeat, m.finalized_at
FROM programs p
LEFT JOIN program_drama_matches m ON m.service_id = p.service_id AND m.start_at = p.start_at
WHERE (sqlc.narg(service_ids)::bigint[] IS NULL OR p.service_id = ANY(sqlc.narg(service_ids)::bigint[]))
  AND p.start_at < @range_end AND p.end_at > @range_start
  AND p.genres -> 0 ->> 'lv1' = '3'
ORDER BY p.start_at, p.service_id;

-- name: ListDramaProgramsToFinalize :many
-- 放送が終わり、まだアーカイブへ移していない、結びつきを確定させていないドラマの番組を返す。
SELECT p.service_id, p.event_id, p.start_at, p.end_at, p.end_undecided, p.name, p.series_episode, p.series_repeat
FROM programs p
LEFT JOIN program_drama_matches m ON m.service_id = p.service_id AND m.start_at = p.start_at
WHERE p.end_at <= @now AND NOT p.end_undecided
  AND p.genres -> 0 ->> 'lv1' = '3' AND m.finalized_at IS NULL
ORDER BY p.start_at, p.service_id;

-- name: GetTMDBSearch :one
SELECT query, results, fetched_at FROM tmdb_searches WHERE query = @query;

-- name: UpsertTMDBSearch :exec
INSERT INTO tmdb_searches (query, results, fetched_at) VALUES (@query, @results, @now)
ON CONFLICT (query) DO UPDATE SET results = excluded.results, fetched_at = excluded.fetched_at;

-- name: GetTMDBSeries :one
SELECT * FROM tmdb_series WHERE id = @id;

-- name: ListTMDBSeries :many
SELECT * FROM tmdb_series WHERE id = ANY(@ids::integer[]);

-- name: UpsertTMDBSeries :exec
INSERT INTO tmdb_series (id, name, original_name, first_air_date, seasons, fetched_at)
VALUES (@id, @name, @original_name, sqlc.narg(first_air_date), @seasons, @now)
ON CONFLICT (id) DO UPDATE SET name = excluded.name, original_name = excluded.original_name,
  first_air_date = excluded.first_air_date, seasons = excluded.seasons, fetched_at = excluded.fetched_at;

-- name: GetTMDBSeason :one
SELECT * FROM tmdb_seasons WHERE series_id = @series_id AND season_number = @season_number;

-- name: UpsertTMDBSeason :exec
INSERT INTO tmdb_seasons (series_id, season_number, episodes, fetched_at) VALUES (@series_id, @season_number, @episodes, @now)
ON CONFLICT (series_id, season_number) DO UPDATE SET episodes = excluded.episodes, fetched_at = excluded.fetched_at;

-- name: GetDramaTitleOverride :one
SELECT * FROM drama_title_overrides WHERE title_key = @title_key;

-- name: ListDramaTitleOverrides :many
SELECT * FROM drama_title_overrides ORDER BY title_key;

-- name: UpsertDramaTitleOverride :exec
INSERT INTO drama_title_overrides (title_key, tmdb_series_id, updated_at) VALUES (@title_key, sqlc.narg(tmdb_series_id), @now)
ON CONFLICT (title_key) DO UPDATE SET tmdb_series_id = excluded.tmdb_series_id, updated_at = excluded.updated_at;

-- name: DeleteDramaTitleOverride :execrows
DELETE FROM drama_title_overrides WHERE title_key = @title_key;

-- name: UnfinalizeDramaMatches :execrows
-- 作品名 title_key の、番組表にある番組の結びつきの確定を外し、次の照合で照合し直させる。管理者が上書きを変えたときに使う。
UPDATE program_drama_matches m SET finalized_at = NULL
WHERE m.title_key = @title_key AND m.finalized_at IS NOT NULL
  AND EXISTS (SELECT 1 FROM programs p WHERE p.service_id = m.service_id AND p.start_at = m.start_at);

-- name: DeleteMovedProgramDramaMatches :exec
-- 放送時間が変わった番組の、前の放送時間の結びつきを消す。アーカイブへ移した、同じイベント ID の番組の結びつきは残す。
DELETE FROM program_drama_matches m
WHERE m.service_id = @service_id AND m.event_id = @event_id AND m.start_at <> @start_at
  AND NOT EXISTS (
    SELECT 1 FROM program_archives a WHERE a.service_id = m.service_id AND a.event_id = m.event_id AND a.start_at = m.start_at
  );

-- name: UpsertProgramDramaMatch :exec
-- 照合した結果を書く。確定させた日時は、一度書いたら消さない (上書きを変えたときは UnfinalizeDramaMatches で外す)。
INSERT INTO program_drama_matches (
  service_id, start_at, event_id, title_key, tmdb_series_id, season_number, episode_number, series_source, episode_source, matched_at, finalized_at
) VALUES (
  @service_id, @start_at, @event_id, @title_key, sqlc.narg(tmdb_series_id), sqlc.narg(season_number), sqlc.narg(episode_number),
  sqlc.narg(series_source), sqlc.narg(episode_source), @now, sqlc.narg(finalized_at)
)
ON CONFLICT (service_id, start_at) DO UPDATE SET
  event_id = excluded.event_id, title_key = excluded.title_key, tmdb_series_id = excluded.tmdb_series_id,
  season_number = excluded.season_number, episode_number = excluded.episode_number, series_source = excluded.series_source,
  episode_source = excluded.episode_source, matched_at = excluded.matched_at,
  finalized_at = COALESCE(program_drama_matches.finalized_at, excluded.finalized_at);

-- name: GetProgramDramaMatch :one
SELECT * FROM program_drama_matches WHERE service_id = @service_id AND start_at = @start_at;

-- name: GetRecordingDramaMatch :one
SELECT * FROM recording_drama_matches WHERE recording_id = @recording_id;

-- name: CopyProgramDramaMatchToRecording :execrows
-- 番組 (service_id, start_at) の結びつきを、録画 recording_id に写す。番組をまだ照合していなければ、何もしない。
INSERT INTO recording_drama_matches (
  recording_id, tmdb_series_id, season_number, episode_number, series_source, episode_source, matched_at
)
SELECT @recording_id, m.tmdb_series_id, m.season_number, m.episode_number, m.series_source, m.episode_source, @now
FROM program_drama_matches m WHERE m.service_id = @service_id AND m.start_at = @start_at
ON CONFLICT (recording_id) DO UPDATE SET
  tmdb_series_id = excluded.tmdb_series_id, season_number = excluded.season_number, episode_number = excluded.episode_number,
  series_source = excluded.series_source, episode_source = excluded.episode_source, matched_at = excluded.matched_at;

-- name: PruneProgramDramaMatches :execrows
-- 番組表からも過去の番組からもなくなった番組の結びつきを消す。照合したばかりの結びつきは、番組を書く途中かもしれないので残す。
DELETE FROM program_drama_matches m
WHERE m.matched_at < @before
  AND NOT EXISTS (SELECT 1 FROM programs p WHERE p.service_id = m.service_id AND p.start_at = m.start_at)
  AND NOT EXISTS (SELECT 1 FROM program_archives a WHERE a.service_id = m.service_id AND a.start_at = m.start_at);

-- name: PruneTMDBSearches :execrows
DELETE FROM tmdb_searches WHERE fetched_at < @before;

-- name: PruneTMDBSeries :execrows
DELETE FROM tmdb_series WHERE fetched_at < @before;

-- name: PruneTMDBSeasons :execrows
DELETE FROM tmdb_seasons WHERE fetched_at < @before;

-- name: ListStaleLinkedTMDBSeries :many
-- 番組か録画に結びついた作品と、ユーザーが見ている作品と見た話の作品 (#1652) のうち、キャッシュが before より前に引いたものか、
-- ないものを、古い順に limit 件まで返す。6 か月で消す前に引き直し、結びついた作品の名前を出し続ける。
-- 見ている作品と見た話には、番組か録画に結びついた作品だけを足せる (dramalibrary) ので、TMDB にない ID は入らない。
SELECT l.id::integer AS id FROM (
  SELECT tmdb_series_id AS id FROM program_drama_matches WHERE tmdb_series_id IS NOT NULL
  UNION
  SELECT tmdb_series_id AS id FROM recording_drama_matches WHERE tmdb_series_id IS NOT NULL
  UNION
  SELECT tmdb_series_id AS id FROM drama_watchlist_entries
  UNION
  SELECT tmdb_series_id AS id FROM drama_watched_episodes
) l
LEFT JOIN tmdb_series s ON s.id = l.id
WHERE s.id IS NULL OR s.fetched_at < @before
ORDER BY s.fetched_at NULLS FIRST, l.id
LIMIT @max_count;

-- name: ListStaleLinkedTMDBSeasons :many
-- 番組か録画に結びついた話のシーズンのうち、キャッシュが before より前に引いたものか、ないものを、古い順に limit 件まで返す。
-- ユーザーが手で見たことにした話 (#1652) のシーズンは、番号を利用者が決めるので (TMDB にないシーズンもありうる)、引き直さない。
SELECT l.series_id::integer AS series_id, l.season_number::integer AS season_number FROM (
  SELECT tmdb_series_id AS series_id, season_number FROM program_drama_matches WHERE episode_number IS NOT NULL
  UNION
  SELECT tmdb_series_id AS series_id, season_number FROM recording_drama_matches WHERE episode_number IS NOT NULL
) l
LEFT JOIN tmdb_seasons s ON s.series_id = l.series_id AND s.season_number = l.season_number
WHERE s.series_id IS NULL OR s.fetched_at < @before
ORDER BY s.fetched_at NULLS FIRST, l.series_id, l.season_number
LIMIT @max_count;

-- name: GetTMDBToken :one
SELECT * FROM tmdb_tokens WHERE id = 1;

-- name: UpsertTMDBToken :exec
INSERT INTO tmdb_tokens (id, token_encrypted, updated_at) VALUES (1, @token_encrypted, @now)
ON CONFLICT (id) DO UPDATE SET token_encrypted = EXCLUDED.token_encrypted, updated_at = EXCLUDED.updated_at;

-- name: DeleteTMDBToken :execrows
DELETE FROM tmdb_tokens WHERE id = 1;

-- name: ListProgramDramaMatchTitleKeys :many
-- 番組の結びつきの作品名の鍵と、番組表かアーカイブにある番組名を、(service_id, start_at) が after より後のものから順に max_count 件返す (#1978)。
-- kotatsu migrate が鍵を作り直すのに使う。番組名のない結びつきは返さない。
SELECT m.service_id, m.start_at, m.title_key, COALESCE(p.name, a.name)::text AS name
FROM program_drama_matches m
LEFT JOIN programs p ON p.service_id = m.service_id AND p.event_id = m.event_id AND p.start_at = m.start_at
LEFT JOIN program_archives a ON a.service_id = m.service_id AND a.event_id = m.event_id AND a.start_at = m.start_at
WHERE (m.service_id, m.start_at) > (@after_service_id::bigint, @after_start_at::timestamptz)
  AND COALESCE(p.name, a.name) IS NOT NULL
ORDER BY m.service_id, m.start_at
LIMIT @max_count;

-- name: RekeyProgramDramaMatch :exec
-- 番組の結びつきの作品名の鍵を書き直し、確定を外して、次の照合で照合し直させる (#1978)。
UPDATE program_drama_matches SET title_key = @title_key, finalized_at = NULL
WHERE service_id = @service_id AND start_at = @start_at;

-- name: CopyDramaTitleOverride :exec
-- 作品名の鍵 from_key の上書きを、鍵 to_key に写す (#1978)。to_key に上書きがあれば、そちらを残す。
INSERT INTO drama_title_overrides (title_key, tmdb_series_id, updated_at)
SELECT @to_key::text, src.tmdb_series_id, src.updated_at FROM drama_title_overrides src WHERE src.title_key = @from_key::text
ON CONFLICT (title_key) DO NOTHING;

-- name: DeleteUnusedDramaTitleOverride :exec
-- 作品名の鍵 key の上書きを、その鍵の番組の結びつきがなければ消す (#1978)。
DELETE FROM drama_title_overrides o
WHERE o.title_key = @key::text
  AND NOT EXISTS (SELECT 1 FROM program_drama_matches m WHERE m.title_key = o.title_key);
