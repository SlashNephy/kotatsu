-- 録り始めに EIT[p/f] で同じ回かを確かめ直す (#1500、設計書 5.1)。

-- name: IsEpisodeRecheckTarget :one
-- 予約の録画を、録り始めに同じ回かを確かめ直す対象かどうか。同じ回を束ねる設定 (#1498) を入れた有効なルールが作ったルール予約で、
-- 結び付いた有効なルールのどれもがその設定を入れているものだけを対象にする。設定を入れていない有効なルールも一致していれば、
-- そのルールが受け持って録るので、見直さない。人が有効に戻した予約 (episode_recheck_waived) も見直さない。
SELECT (r.kind = 'rule' AND r.event_id IS NOT NULL AND NOT r.episode_recheck_waived
  AND EXISTS (
    SELECT 1 FROM reservation_rules rr JOIN rules ru ON ru.id = rr.rule_id
    WHERE rr.reservation_id = r.id AND ru.enabled AND ru.bundle_same_episodes
  )
  AND NOT EXISTS (
    SELECT 1 FROM reservation_rules rr JOIN rules ru ON ru.id = rr.rule_id
    WHERE rr.reservation_id = r.id AND ru.enabled AND NOT ru.bundle_same_episodes
  ))::boolean AS target
FROM reservations r
WHERE r.id = @id;

-- name: ListEarlierRecordedEpisodes :many
-- 録り始めた録画 (api_id) と比べる「録った回」(#1500)。録画履歴のうち @since より後に始まったものと、ほかの録画ソフトから
-- 引き継いだもの (期間で切らない)、それに、この録画より先に作った録画中の録画。録画履歴は、正常に終了し、ドロップが閾値以下で、
-- スクランブルの警告のない録画だけを持つ (設計書 4.4)。同じ番組の録画 (取り消して戻した続きなど) は除く。
-- 先に作った録画に限るのは、同時に始まった同じ回の録画が、互いを録画済みとみなして両方やめないためである。api_id は録画を作るときに
-- 振る番号で、EPGStation から取り込んだ録画は取り込み元の ID を引き継ぐが、取り込むのは録り終えた録画だけなので、録画中の録画では作った順になる。
SELECT h.service_id, h.name, h.description, h.series_episode, h.syobocal_tid, h.anime_count
FROM recording_history h
LEFT JOIN recordings hr ON hr.id = h.recording_id
WHERE (h.imported OR h.start_at >= @since)
  AND (hr.id IS NULL OR NOT (hr.service_id = @service_id AND hr.event_id IS NOT DISTINCT FROM sqlc.narg(event_id)::bigint))
UNION ALL
SELECT rec.service_id, rec.name, rec.description, p.series_episode, a.syobocal_tid, a.count_start AS anime_count
FROM recordings rec
LEFT JOIN programs p ON p.service_id = rec.service_id AND p.event_id = rec.event_id
LEFT JOIN LATERAL (
  SELECT min(e.syobocal_tid)::integer AS syobocal_tid, min(e.count_start)::double precision AS count_start
  FROM recording_anime_episodes e
  WHERE e.recording_id = rec.id
  HAVING count(*) = 1 AND min(e.count_start) = min(e.count_end)
) a ON true
WHERE rec.state = 'recording' AND rec.api_id < @api_id AND rec.start_at >= @since
  AND NOT (rec.service_id = @service_id AND rec.event_id IS NOT DISTINCT FROM sqlc.narg(event_id)::bigint);

-- name: GetRecordingEpisodeSource :one
-- 録画の回を読むための、番組のシリーズの記述子の話数と、録画が 1 つの話に結びついたときの TID と話数。
SELECT p.series_episode, s.channel_type, e.syobocal_tid, e.count_start AS anime_count
FROM recordings rec
JOIN services s ON s.id = rec.service_id
LEFT JOIN programs p ON p.service_id = rec.service_id AND p.event_id = rec.event_id
-- 録画の話が 1 つだけで、1 話分のときに限る (ListRecordedEpisodes とそろえる)。
LEFT JOIN recording_anime_episodes e ON e.recording_id = rec.id AND e.count_start = e.count_end
  AND NOT EXISTS (SELECT 1 FROM recording_anime_episodes o WHERE o.recording_id = rec.id AND o.position <> e.position)
WHERE rec.id = @id;

-- name: RewriteRecordingProgramInfo :exec
-- 録画の番組名、概要、詳細を、EIT[p/f] で読んだもので書き直す (#1500、#1646)。空の値は渡さず、前の値を残す。
UPDATE recordings SET
  name = coalesce(sqlc.narg(name)::text, name),
  description = coalesce(sqlc.narg(description)::text, description),
  extended = coalesce(sqlc.narg(extended)::jsonb, extended),
  name_normalized = coalesce(sqlc.narg(name_normalized)::text, name_normalized),
  description_normalized = coalesce(sqlc.narg(description_normalized)::text, description_normalized),
  updated_at = @now
WHERE id = @id;

-- name: GetProgramForRecheck :one
-- EIT[p/f] で書き直す前の、番組表の番組の正規化した番組名、概要、詳細と、シリーズの記述子の話数 (#1500)。
SELECT name_normalized, description_normalized, extended_normalized, series_episode, series_repeat
FROM programs
WHERE service_id = @service_id AND event_id = @event_id;

-- name: RewriteProgramInfo :execrows
-- 番組表の番組を、EIT[p/f] で読んだもので書き直す (#1500)。空の値は渡さず、前の値を残す。シリーズの記述子は、
-- EIT[p/f] が載せていたときだけ (series_set)、その値 (話数が 0 なら NULL) で書き直す。時刻は書かない。
UPDATE programs SET
  name = coalesce(sqlc.narg(name)::text, name),
  description = coalesce(sqlc.narg(description)::text, description),
  name_normalized = coalesce(sqlc.narg(name_normalized)::text, name_normalized),
  description_normalized = coalesce(sqlc.narg(description_normalized)::text, description_normalized),
  extended = coalesce(sqlc.narg(extended)::jsonb, extended),
  extended_normalized = coalesce(sqlc.narg(extended_normalized)::text, extended_normalized),
  series_episode = CASE WHEN @series_set::boolean THEN sqlc.narg(series_episode)::integer ELSE series_episode END,
  series_repeat = CASE WHEN @series_set::boolean THEN sqlc.narg(series_repeat)::boolean ELSE series_repeat END,
  updated_at = @now
WHERE service_id = @service_id AND event_id = @event_id;

-- name: DisableReservationForRecordedEpisode :execrows
-- 録り始めに同じ回を録画済みと分かった予約を、理由を付けて無効にする (#1500)。有効なままのときだけ変える。
-- 録画の行を消しても、予約を有効のまま残すと、スケジューラがまた録り始めるからである。
UPDATE reservations SET enabled = false, disabled_reason = 'recorded_episode', disabled_service_id = sqlc.narg(service_id)::bigint, updated_at = @now
WHERE id = @id AND enabled;
