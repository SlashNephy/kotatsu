-- name: SearchProgramCandidates :many
-- 番組の検索の候補。チャンネル種別、サービス、ジャンルの大分類で絞り、開始時刻、サービス、eventId の順に、カーソルの後ろから返す。
-- ジャンルの中分類とキーワードは、ルールの照合と同じ照合関数で、Go で判定する (設計書 3.5)。
-- include_archive なら、開始時刻が [archive_from, archive_to) のアーカイブの番組も含める。
SELECT c.archived, c.service_id, c.event_id, c.start_at, c.end_at, c.end_undecided, c.is_free,
  c.name, c.description, c.extended, c.genres, c.channel_type
FROM (
  SELECT false AS archived, p.service_id, p.event_id, p.start_at, p.end_at, p.end_undecided, p.is_free,
    p.name, p.description, p.extended, p.genres, s.channel_type
  FROM programs p
  JOIN services s ON s.id = p.service_id
  UNION ALL
  SELECT true AS archived, a.service_id, a.event_id, a.start_at, a.end_at, a.end_undecided, a.is_free,
    a.name, a.description, a.extended, a.genres, s.channel_type
  FROM program_archives a
  JOIN services s ON s.id = a.service_id
  WHERE @include_archive::boolean AND a.start_at >= @archive_from::timestamptz AND a.start_at < @archive_to::timestamptz
) c
WHERE (
    (cardinality(@channel_types::text[]) = 0 AND cardinality(@service_ids::bigint[]) = 0)
    OR c.channel_type = ANY(@channel_types::text[])
    OR c.service_id = ANY(@service_ids::bigint[])
  )
  AND (
    cardinality(@genre_lv1s::bigint[]) = 0
    OR EXISTS (SELECT 1 FROM jsonb_array_elements(c.genres) g WHERE (g ->> 'lv1')::bigint = ANY(@genre_lv1s::bigint[]))
  )
  AND NOT EXISTS (
    SELECT 1 FROM jsonb_array_elements(c.genres) g WHERE (g ->> 'lv1')::bigint = ANY(@excluded_genre_lv1s::bigint[])
  )
  AND (c.start_at, c.service_id, c.event_id) > (@after_start_at::timestamptz, @after_service_id::bigint, @after_event_id::bigint)
ORDER BY c.start_at, c.service_id, c.event_id
LIMIT @batch_size::integer;
