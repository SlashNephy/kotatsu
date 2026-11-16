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
