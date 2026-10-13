-- name: ListChannelTypes :many
SELECT type, display_name, sort_order FROM channel_types ORDER BY sort_order, type;

-- name: ListChannelTypesByTypes :many
SELECT type, display_name, sort_order FROM channel_types WHERE type = ANY(@types::text[]);

-- name: ListGuideServices :many
-- 番組表の並び順のサービス。チャンネル種別の並び順、管理者の上書き、リモコンのキー、サービス ID の順に並べる。
-- media_service_types を渡すと、その種別 (映像か音声を流すサービス) だけを返す。空なら、種別で絞らない。
SELECT s.id, s.network_id, s.sid, s.name, s.channel_type, s.channel, s.remote_control_key_id, s.logo_id, s.hidden, s.sort_order
FROM services s
JOIN channel_types ct ON ct.type = s.channel_type
WHERE (@include_hidden::boolean OR NOT s.hidden)
  AND (cardinality(@media_service_types::bigint[]) = 0 OR s.service_type = ANY(@media_service_types::bigint[]))
  AND (cardinality(@ids::bigint[]) = 0 OR s.id = ANY(@ids::bigint[]))
ORDER BY ct.sort_order, ct.type, s.sort_order NULLS LAST, s.remote_control_key_id NULLS LAST, s.id;

-- name: ListProgramsInRange :many
-- 時間帯 [from, to) に重なる番組。
SELECT service_id, event_id, start_at, end_at, end_undecided, is_free, name, description, extended, genres, audios,
  related_items
FROM programs
WHERE service_id = ANY(@service_ids::bigint[]) AND end_at > @from_at AND start_at < @to_at
ORDER BY service_id, start_at, event_id;

-- name: GetGuideProgram :one
SELECT service_id, event_id, start_at, end_at, end_undecided, is_free, name, description, extended, genres, audios,
  related_items
FROM programs
WHERE service_id = @service_id AND event_id = @event_id;

-- name: GetArchivedProgram :one
SELECT service_id, event_id, start_at, end_at, end_undecided, is_free, name, description, extended, genres, audios,
  related_items
FROM program_archives
WHERE service_id = @service_id AND event_id = @event_id AND start_at = @start_at;
