-- name: GetRecordingNextEpisodeSource :one
-- 録画の続きの放送 (#1564) を探すため、録画の回を読む元を返す。番組名、概要、詳細と、番組 (番組表かアーカイブ) の
-- シリーズの記述子の話数と、録画が 1 つの話 (1 話分) に結びついたときの TID と話数。録画履歴に写すもの (設計書 4.4) と揃える。
SELECT rec.name, rec.description, rec.extended,
  p.series_episode AS program_series_episode, pa.series_episode AS archived_series_episode,
  e.syobocal_tid, e.count_start
FROM recordings rec
LEFT JOIN programs p ON p.service_id = rec.service_id AND p.event_id = rec.event_id
LEFT JOIN program_archives pa ON pa.service_id = rec.service_id AND pa.event_id = rec.event_id AND pa.start_at = rec.start_at
LEFT JOIN recording_anime_episodes e ON e.recording_id = rec.id AND e.count_start = e.count_end
  AND NOT EXISTS (SELECT 1 FROM recording_anime_episodes o WHERE o.recording_id = rec.id AND o.position <> e.position)
WHERE rec.id = @id;

-- name: ListSeriesNextPrograms :many
-- シリーズ名が series で、話数が number より大きく、since から until までに始まる、終了時刻の決まった番組を返す (#1564)。
SELECT service_id, event_id, start_at, episode_number::double precision AS episode_number, related_items
FROM programs
WHERE series = @series::text AND episode_number IS NOT NULL AND episode_number > @number::bigint
  AND start_at >= @since AND start_at < @until AND NOT end_undecided
ORDER BY start_at, service_id, event_id
LIMIT @max_rows::integer;

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

-- name: ListRecordedSeriesEpisodes :many
-- シリーズ名が series の録画履歴の、回を読む元 (#1564)。録った回を、続きの放送から外すのに使う。
SELECT name, description, series_episode FROM recording_history WHERE series = @series::text;

-- name: ListRecordedAnimeCounts :many
-- TID が syobocal_tid の話に結びついた録画履歴の話数 (#1564)。録った回を、続きの放送から外すのに使う。
SELECT DISTINCT anime_count::double precision AS anime_count
FROM recording_history
WHERE syobocal_tid = @syobocal_tid::integer AND anime_count IS NOT NULL;

-- name: ListWatchedNextEpisodeSources :many
-- ユーザー user_id が since より後に、録画の長さの min_ratio 以上まで再生した録画の、回を読む元を、最後に見た新しい順に返す (#1564)。
-- ホームの「続きの放送」で、見たシリーズを決めるのに使う。回を読む元は GetRecordingNextEpisodeSource と揃える。
-- 録画中の録画と、長さの分からない録画は、見た割合を出せないので外す。
SELECT rec.id, rec.name, rec.description, rec.extended,
  p.series_episode AS program_series_episode, pa.series_episode AS archived_series_episode,
  e.syobocal_tid, e.count_start, w.updated_at
FROM watch_progress w
JOIN recordings rec ON rec.id = w.recording_id
LEFT JOIN programs p ON p.service_id = rec.service_id AND p.event_id = rec.event_id
LEFT JOIN program_archives pa ON pa.service_id = rec.service_id AND pa.event_id = rec.event_id AND pa.start_at = rec.start_at
LEFT JOIN recording_anime_episodes e ON e.recording_id = rec.id AND e.count_start = e.count_end
  AND NOT EXISTS (SELECT 1 FROM recording_anime_episodes o WHERE o.recording_id = rec.id AND o.position <> e.position)
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
