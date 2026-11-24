-- name: GetRecordingNextEpisodeSource :one
-- 録画の続きの放送 (#1564) を探すため、録画の回を読む元を返す。番組名、概要、詳細と、番組 (番組表かアーカイブ) の
-- シリーズの記述子の話数と、録画が 1 つの話 (1 話分) に結びついたときの TID と話数と、録画が結びついた TMDB の作品とシーズンと話の番号。
-- 録画履歴にコピーするもの (設計書 4.4) と揃える。
SELECT rec.name, rec.description, rec.extended,
  p.series_episode AS program_series_episode, pa.series_episode AS archived_series_episode,
  e.syobocal_tid, e.count_start, d.tmdb_series_id, d.season_number AS tmdb_season_number, d.episode_number AS tmdb_episode_number
FROM recordings rec
LEFT JOIN programs p ON p.service_id = rec.service_id AND p.event_id = rec.event_id
LEFT JOIN program_archives pa ON pa.service_id = rec.service_id AND pa.event_id = rec.event_id AND pa.start_at = rec.start_at
LEFT JOIN recording_anime_episodes e ON e.recording_id = rec.id AND e.count_start = e.count_end
  AND NOT EXISTS (SELECT 1 FROM recording_anime_episodes o WHERE o.recording_id = rec.id AND o.position <> e.position)
LEFT JOIN recording_drama_matches d ON d.recording_id = rec.id
WHERE rec.id = @id;

-- name: ListAnimeNextPrograms :many
-- しょぼいカレンダーの TID に結びついた番組のうち、話数が count より大きく、since から until までに始まる、
-- 終了時刻の決まった番組を返す (#1564)。2 話連続の放送のように 1 つの番組が複数の話に結びついたら、小さい話数で 1 行にする。
SELECT p.service_id, p.event_id, p.start_at, min(e.count_start)::double precision AS episode_number, p.related_items
FROM program_anime_episodes e
JOIN program_anime_matches m ON m.service_id = e.service_id AND m.start_at = e.start_at
JOIN programs p ON p.service_id = m.service_id AND p.event_id = m.event_id AND p.start_at = m.start_at
WHERE e.syobocal_tid = @syobocal_tid::integer AND e.count_start > @count::double precision
  AND p.start_at >= @since AND p.start_at < @until AND NOT p.end_undecided
GROUP BY p.service_id, p.event_id, p.start_at, p.related_items
ORDER BY p.start_at, p.service_id, p.event_id
LIMIT @max_rows::integer;

-- name: ListDramaNextPrograms :many
-- TMDB の作品 tmdb_series_id のシーズン season_number の話に結びついた番組のうち、話の番号が episode_number より大きく、
-- since から until までに始まる、終了時刻の決まった番組を返す (#1564)。
SELECT p.service_id, p.event_id, p.start_at, m.episode_number::double precision AS episode_number, p.related_items
FROM program_drama_matches m
JOIN programs p ON p.service_id = m.service_id AND p.event_id = m.event_id AND p.start_at = m.start_at
WHERE m.tmdb_series_id = @tmdb_series_id::integer AND m.season_number = @season_number::integer
  AND m.episode_number > @episode_number::integer
  AND p.start_at >= @since AND p.start_at < @until AND NOT p.end_undecided
ORDER BY p.start_at, p.service_id, p.event_id
LIMIT @max_rows::integer;

-- name: ListRecordedAnimeCounts :many
-- TID が syobocal_tid の話に結びついた録画履歴の話数 (#1564)。録った回を、続きの放送から外すのに使う。
SELECT DISTINCT anime_count::double precision AS anime_count
FROM recording_history
WHERE syobocal_tid = @syobocal_tid::integer AND anime_count IS NOT NULL;

-- name: ListRecordedDramaEpisodes :many
-- TMDB の作品 tmdb_series_id のシーズン season_number の話に結びついた録画履歴の話の番号 (#1564)。録った回を、続きの放送から外すのに使う。
SELECT DISTINCT tmdb_episode_number::double precision AS episode_number
FROM recording_history
WHERE tmdb_series_id = @tmdb_series_id::integer AND tmdb_season_number = @season_number::integer AND tmdb_episode_number IS NOT NULL;

-- name: ListWatchedNextEpisodeSources :many
-- ユーザー user_id が since より後に、録画の長さの min_ratio 以上まで再生した録画の、回を読む元を、最後に見た新しい順に返す (#1564)。
-- ホームの「続きの放送」で、見たシリーズを決めるのに使う。回を読む元は GetRecordingNextEpisodeSource と揃える。
-- 録画中の録画と、長さの分からない録画は、見た割合を出せないので外す。
SELECT rec.id, rec.name, rec.description, rec.extended,
  p.series_episode AS program_series_episode, pa.series_episode AS archived_series_episode,
  e.syobocal_tid, e.count_start, d.tmdb_series_id, d.season_number AS tmdb_season_number, d.episode_number AS tmdb_episode_number,
  w.updated_at
FROM watch_progress w
JOIN recordings rec ON rec.id = w.recording_id
LEFT JOIN programs p ON p.service_id = rec.service_id AND p.event_id = rec.event_id
LEFT JOIN program_archives pa ON pa.service_id = rec.service_id AND pa.event_id = rec.event_id AND pa.start_at = rec.start_at
LEFT JOIN recording_anime_episodes e ON e.recording_id = rec.id AND e.count_start = e.count_end
  AND NOT EXISTS (SELECT 1 FROM recording_anime_episodes o WHERE o.recording_id = rec.id AND o.position <> e.position)
LEFT JOIN recording_drama_matches d ON d.recording_id = rec.id
WHERE w.user_id = @user_id AND w.updated_at >= @since
  AND rec.recorded_from IS NOT NULL AND rec.recorded_until IS NOT NULL AND rec.recorded_until > rec.recorded_from
  AND w.position_ms >= extract(epoch FROM (rec.recorded_until - rec.recorded_from)) * 1000 * @min_ratio::double precision
ORDER BY w.updated_at DESC, rec.id
LIMIT @max_rows::integer;

-- name: ListSeriesNextProgramsBatch :many
-- シリーズ名 series[i] ごとに、話数が numbers[i] より大きく、since から until までに始まる、終了時刻の決まった番組を、
-- 放送の早い順に max_rows 件まで返す (#1564)。ホームの「続きの放送」で、見たシリーズをまとめて引く。
SELECT k.series::text AS key_series, c.service_id, c.event_id, c.start_at, c.episode_number, c.related_items
FROM (SELECT unnest(@series::text[]) AS series, unnest(@numbers::bigint[]) AS number) AS k
CROSS JOIN LATERAL (
  SELECT p.service_id, p.event_id, p.start_at, p.episode_number::double precision AS episode_number, p.related_items
  FROM programs p
  WHERE p.series = k.series AND p.episode_number IS NOT NULL AND p.episode_number > k.number
    AND p.start_at >= @since AND p.start_at < @until AND NOT p.end_undecided
  ORDER BY p.start_at, p.service_id, p.event_id
  LIMIT @max_rows::integer
) c
ORDER BY k.series, c.start_at, c.service_id, c.event_id;

-- name: ListAnimeNextProgramsBatch :many
-- しょぼいカレンダーの TID tids[i] ごとに、話数が counts[i] より大きく、since から until までに始まる、終了時刻の決まった番組を、
-- 放送の早い順に max_rows 件まで返す (#1564)。1 つの番組が複数の話に結びついたら、小さい話数で 1 行にする (ListAnimeNextPrograms と同じ)。
SELECT k.tid::integer AS key_tid, c.service_id, c.event_id, c.start_at, c.episode_number, c.related_items
FROM (SELECT unnest(@tids::integer[]) AS tid, unnest(@counts::double precision[]) AS count) AS k
CROSS JOIN LATERAL (
  SELECT p.service_id, p.event_id, p.start_at, min(e.count_start)::double precision AS episode_number, p.related_items
  FROM program_anime_episodes e
  JOIN program_anime_matches m ON m.service_id = e.service_id AND m.start_at = e.start_at
  JOIN programs p ON p.service_id = m.service_id AND p.event_id = m.event_id AND p.start_at = m.start_at
  WHERE e.syobocal_tid = k.tid AND e.count_start > k.count
    AND p.start_at >= @since AND p.start_at < @until AND NOT p.end_undecided
  GROUP BY p.service_id, p.event_id, p.start_at, p.related_items
  ORDER BY p.start_at, p.service_id, p.event_id
  LIMIT @max_rows::integer
) c
ORDER BY k.tid, c.start_at, c.service_id, c.event_id;

-- name: ListRecordedSeriesEpisodesBatch :many
-- シリーズ名が series のどれかの録画履歴の、回を読む元 (#1564)。ListRecordedSeriesEpisodes をまとめて引く。
SELECT series::text AS key_series, name, description, series_episode
FROM recording_history WHERE series = ANY(@series::text[]);

-- name: ListRecordedAnimeCountsBatch :many
-- TID が tids のどれかの話に結びついた録画履歴の TID と話数 (#1564)。ListRecordedAnimeCounts をまとめて引く。
SELECT DISTINCT syobocal_tid::integer AS key_tid, anime_count::double precision AS anime_count
FROM recording_history
WHERE syobocal_tid = ANY(@tids::integer[]) AND anime_count IS NOT NULL;

-- name: ListDramaNextProgramsBatch :many
-- TMDB の作品 series_ids[i] のシーズン seasons[i] ごとに、話の番号が episodes[i] より大きく、since から until までに始まる、
-- 終了時刻の決まった番組を、放送の早い順に max_rows 件まで返す (#1564)。ListDramaNextPrograms をまとめて引く。
SELECT k.series_id::integer AS key_series_id, k.season::integer AS key_season,
  c.service_id, c.event_id, c.start_at, c.episode_number, c.related_items
FROM (
  SELECT unnest(@series_ids::integer[]) AS series_id, unnest(@seasons::integer[]) AS season, unnest(@episodes::integer[]) AS episode
) AS k
CROSS JOIN LATERAL (
  SELECT p.service_id, p.event_id, p.start_at, m.episode_number::double precision AS episode_number, p.related_items
  FROM program_drama_matches m
  JOIN programs p ON p.service_id = m.service_id AND p.event_id = m.event_id AND p.start_at = m.start_at
  WHERE m.tmdb_series_id = k.series_id AND m.season_number = k.season AND m.episode_number > k.episode
    AND p.start_at >= @since AND p.start_at < @until AND NOT p.end_undecided
  ORDER BY p.start_at, p.service_id, p.event_id
  LIMIT @max_rows::integer
) c
ORDER BY k.series_id, k.season, c.start_at, c.service_id, c.event_id;

-- name: ListRecordedDramaEpisodesBatch :many
-- TMDB の作品 series_ids[i] のシーズン seasons[i] のどれかの話に結びついた録画履歴の、作品とシーズンと話の番号 (#1564)。
-- ListRecordedDramaEpisodes をまとめて引く。
SELECT DISTINCT h.tmdb_series_id::integer AS key_series_id, h.tmdb_season_number::integer AS key_season,
  h.tmdb_episode_number::double precision AS episode_number
FROM recording_history h
JOIN (SELECT unnest(@series_ids::integer[]) AS series_id, unnest(@seasons::integer[]) AS season) AS k
  ON h.tmdb_series_id = k.series_id AND h.tmdb_season_number = k.season
WHERE h.tmdb_episode_number IS NOT NULL;

-- name: ListSeriesFamilyNextProgramsBatch :many
-- シリーズ名が作品名 stems[i] で始まり (作品名そのものは除く)、話数が numbers[i] より大きく、since から until までに始まる、
-- 終了時刻の決まった番組を、放送の早い順に返す (#2023)。番組名にサブタイトルが入り、シリーズ名が回ごとに変わる作品の回を探す。
-- 作品名で始まるだけでは同じ作品とは限らないので、呼ぶ側が番組名を読み直して確認する。確認する前に数を絞ると、
-- 作品名で始まる別の番組 (続編や、作品名を含む長い名前の番組) が先に並んだときに、同じ作品の回を落とすので、件数は絞らない。
SELECT k.stem::text AS key_stem, p.service_id, p.event_id, p.start_at, p.episode_number::double precision AS episode_number,
  p.related_items, p.name
FROM (SELECT unnest(@stems::text[]) AS stem, unnest(@numbers::bigint[]) AS number) AS k
JOIN programs p ON starts_with(p.series, k.stem) AND p.series <> k.stem AND p.episode_number IS NOT NULL AND p.episode_number > k.number
WHERE p.start_at >= @since AND p.start_at < @until AND NOT p.end_undecided
ORDER BY k.stem, p.start_at, p.service_id, p.event_id;

-- name: ListRecordedSeriesFamilyEpisodesBatch :many
-- シリーズ名が作品名 stems のどれかで始まる (作品名そのものは除く) 録画履歴の、回を読む元 (#2023)。
-- 作品名で始まるだけでは同じ作品とは限らないので、呼ぶ側が番組名を読み直して確認する。
SELECT k.stem::text AS key_stem, h.name, h.description, h.series_episode
FROM unnest(@stems::text[]) AS k(stem)
JOIN recording_history h ON starts_with(h.series, k.stem) AND h.series <> k.stem;

-- name: ListKnownSeries :many
-- series のうち、話数のある番組表の番組か、録画履歴のシリーズ名にあるもの (#2023)。サブタイトルのない回があるかを確認する。
SELECT p.series::text AS series FROM programs p WHERE p.series = ANY(@series::text[]) AND p.episode_number IS NOT NULL
UNION
SELECT h.series::text AS series FROM recording_history h WHERE h.series = ANY(@series::text[]);
