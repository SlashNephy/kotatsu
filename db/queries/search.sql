-- name: SearchProgramCandidates :many
-- 番組の検索の候補。映像か音声を流すサービス (media_service_types) の番組だけを、チャンネル種別、サービス、ジャンルの大分類で絞り、開始時刻、サービス、eventId の順に、カーソルの後ろから返す。
-- ジャンルの中分類とキーワードは、ルールの照合と同じ照合関数で、Go で判定する (設計書 3.5)。
-- include_archive なら、開始時刻が [archive_from, archive_to) の放送の終わった番組 (アーカイブと、まだ移していない番組) も含める。
-- キーワードのうち、正規表現でなく、番組名と概要だけを見るものは、正規化した列の LIKE で粗く絞る (keyword_*)。
-- 最終的な判定は Go の照合関数で行うので、ここでは一致しうる番組を落とさないことだけを守る。
SELECT c.archived, c.service_id, c.event_id, c.start_at, c.end_at, c.end_undecided, c.is_free,
  c.name, c.description, c.extended, c.genres, c.audios, c.related_items, c.channel_type
FROM (
  SELECT false AS archived, p.service_id, p.event_id, p.start_at, p.end_at, p.end_undecided, p.is_free,
    p.name, p.description, p.extended, p.genres, p.audios, p.related_items, s.channel_type,
    p.name_normalized, p.description_normalized
  FROM programs p
  JOIN services s ON s.id = p.service_id
  -- programs には、アーカイブへ移すまでの猶予の間、放送の終わった番組も残る。
  -- 過去の放送を含めないときは、まだ終わっていない番組だけにし、含めるときは、終わった番組も期間で絞る。
  WHERE s.service_type = ANY(@media_service_types::bigint[])
    AND (p.end_undecided OR p.end_at > @now::timestamptz
      OR (@include_archive::boolean AND p.start_at >= @archive_from::timestamptz AND p.start_at < @archive_to::timestamptz))
  UNION ALL
  SELECT true AS archived, a.service_id, a.event_id, a.start_at, a.end_at, a.end_undecided, a.is_free,
    a.name, a.description, a.extended, a.genres, a.audios, a.related_items, s.channel_type,
    a.name_normalized, a.description_normalized
  FROM program_archives a
  JOIN services s ON s.id = a.service_id
  WHERE @include_archive::boolean AND a.start_at >= @archive_from::timestamptz AND a.start_at < @archive_to::timestamptz
    AND s.service_type = ANY(@media_service_types::bigint[])
) c
WHERE (
    (cardinality(@channel_types::text[]) = 0 AND cardinality(@service_ids::bigint[]) = 0)
    OR c.channel_type = ANY(@channel_types::text[])
    OR c.service_id = ANY(@service_ids::bigint[])
  )
  AND NOT c.service_id = ANY(@excluded_service_ids::bigint[])
  AND (
    cardinality(@genre_lv1s::bigint[]) = 0
    OR EXISTS (SELECT 1 FROM jsonb_array_elements(c.genres) g WHERE (g ->> 'lv1')::bigint = ANY(@genre_lv1s::bigint[]))
  )
  AND NOT EXISTS (
    SELECT 1 FROM jsonb_array_elements(c.genres) g WHERE (g ->> 'lv1')::bigint = ANY(@excluded_genre_lv1s::bigint[])
  )
  AND (NOT @free_only::boolean OR c.is_free)
  AND NOT EXISTS (
    SELECT 1
    FROM generate_subscripts(@keyword_patterns::text[], 1) AS k (i)
    WHERE NOT (
      ((@keyword_in_name::boolean[])[k.i] AND CASE WHEN (@keyword_case_sensitive::boolean[])[k.i]
        THEN c.name_normalized LIKE (@keyword_patterns::text[])[k.i] ELSE c.name_normalized ILIKE (@keyword_patterns::text[])[k.i] END)
      OR ((@keyword_in_description::boolean[])[k.i] AND CASE WHEN (@keyword_case_sensitive::boolean[])[k.i]
        THEN c.description_normalized LIKE (@keyword_patterns::text[])[k.i] ELSE c.description_normalized ILIKE (@keyword_patterns::text[])[k.i] END)
    )
  )
  AND (c.start_at, c.service_id, c.event_id) > (@after_start_at::timestamptz, @after_service_id::bigint, @after_event_id::bigint)
ORDER BY c.start_at, c.service_id, c.event_id
LIMIT @batch_size::integer;
