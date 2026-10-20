-- EPGStation 互換の API (#431、設計書 9.8) で返す録画。セグメントがなく、再生できない録画は返さない。

-- name: ListAPIRecordings :many
-- 録画を、録画を始めた日時の新しい順に、offset から max_count 件返す。keyword が空でなければ、番組名で絞る。
SELECT r.api_id, r.id, r.service_id, r.name, r.description, r.start_at, r.end_at, r.genres, r.state, r.protected,
  coalesce(res.extended, '{}'::jsonb)::jsonb AS extended,
  (SELECT coalesce(sum(f.size), 0) FROM recording_files f WHERE f.recording_id = r.id AND f.kind = 'segment')::bigint AS size,
  -- 代表画像を置くセグメント (書いたものがある最初のセグメント。thumbnail.ImageSegment と同じ)。なければ全て 0 の ID。
  coalesce((SELECT f.id FROM recording_files f WHERE f.recording_id = r.id AND f.kind = 'segment' AND f.size > 0
    ORDER BY f.started_at, f.id LIMIT 1), '00000000-0000-0000-0000-000000000000'::uuid)::uuid AS image_segment_id
FROM recordings r
LEFT JOIN reservations res ON res.id = r.reservation_id
WHERE EXISTS (SELECT 1 FROM recording_files f WHERE f.recording_id = r.id AND f.kind = 'segment')
  AND (@keyword::text = '' OR r.name ILIKE '%' || @keyword::text || '%')
ORDER BY r.recorded_from DESC, r.id DESC
LIMIT @max_count OFFSET @skip;

-- name: CountAPIRecordings :one
SELECT count(*) FROM recordings r
WHERE EXISTS (SELECT 1 FROM recording_files f WHERE f.recording_id = r.id AND f.kind = 'segment')
  AND (@keyword::text = '' OR r.name ILIKE '%' || @keyword::text || '%');

-- name: GetAPIRecording :one
SELECT r.api_id, r.id, r.service_id, r.name, r.description, r.start_at, r.end_at, r.genres, r.state, r.protected,
  coalesce(res.extended, '{}'::jsonb)::jsonb AS extended,
  (SELECT coalesce(sum(f.size), 0) FROM recording_files f WHERE f.recording_id = r.id AND f.kind = 'segment')::bigint AS size,
  -- 代表画像を置くセグメント (書いたものがある最初のセグメント。thumbnail.ImageSegment と同じ)。なければ全て 0 の ID。
  coalesce((SELECT f.id FROM recording_files f WHERE f.recording_id = r.id AND f.kind = 'segment' AND f.size > 0
    ORDER BY f.started_at, f.id LIMIT 1), '00000000-0000-0000-0000-000000000000'::uuid)::uuid AS image_segment_id
FROM recordings r
LEFT JOIN reservations res ON res.id = r.reservation_id
WHERE r.api_id = @api_id
  AND EXISTS (SELECT 1 FROM recording_files f WHERE f.recording_id = r.id AND f.kind = 'segment');

-- name: GetRecordingIDByAPIID :one
SELECT id FROM recordings WHERE api_id = @api_id;
