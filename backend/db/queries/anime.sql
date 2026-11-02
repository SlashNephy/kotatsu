-- name: GetAnimeImport :one
SELECT * FROM anime_imports WHERE source = @source;

-- name: ListAnimeImports :many
SELECT * FROM anime_imports ORDER BY source;

-- name: RecordAnimeImport :exec
-- 取り込めたことを記録する。内容が変わっていなければ (304)、呼ぶ側が前の ETag と件数を渡す。
INSERT INTO anime_imports (source, etag, checked_at, imported_at, item_count, error)
VALUES (@source, sqlc.narg(etag), @now, @now, @item_count, NULL)
ON CONFLICT (source) DO UPDATE SET
  etag = excluded.etag, checked_at = excluded.checked_at, imported_at = excluded.imported_at,
  item_count = excluded.item_count, error = NULL;

-- name: RecordAnimeImportFailure :exec
-- 取り込めなかった理由を記録する。前に取り込めた内容と ETag は残し、1 時間後にまた問い合わせる。
INSERT INTO anime_imports (source, etag, checked_at, imported_at, item_count, error)
VALUES (@source, NULL, @now, NULL, 0, @error)
ON CONFLICT (source) DO UPDATE SET checked_at = excluded.checked_at, error = excluded.error;

-- name: LockAnimeImport :exec
-- 取り込み元 source の入れ替えを、1 つずつ行わせる。トランザクションが終わるまでロックする。
-- 取り込みのジョブが重なると、後の入れ替えが、先に入れた行と主キーでぶつかり、取り込めなかったと記録してしまうため。
SELECT pg_advisory_xact_lock(hashtext('kotatsu.anime_import'), hashtext(@source::text));

-- name: DeleteAnimeChannelDefinitions :exec
DELETE FROM anime_channel_definitions;

-- name: InsertAnimeChannelDefinitions :exec
-- 同じサービスは 1 度だけ渡す (先に出てきたものを使うのは、呼ぶ側で決める)。配列は NULL を持てないので、対応のない ID は 0 で渡す。
INSERT INTO anime_channel_definitions (channel_type, network_id, sid, name, syobocal_channel_id, annict_channel_id)
SELECT t.channel_type, t.network_id, t.sid, t.name, NULLIF(t.syobocal_channel_id, 0), NULLIF(t.annict_channel_id, 0)
FROM (
  SELECT unnest(@channel_types::text[]) AS channel_type, unnest(@network_ids::bigint[]) AS network_id,
    unnest(@sids::bigint[]) AS sid, unnest(@names::text[]) AS name,
    unnest(@syobocal_channel_ids::integer[]) AS syobocal_channel_id, unnest(@annict_channel_ids::integer[]) AS annict_channel_id
) AS t;

-- name: DeleteAnimeWorks :exec
DELETE FROM anime_works WHERE source = @source;

-- name: InsertAnimeWorks :exec
-- 配列は NULL を持てないので、対応のない ID は 0 で渡す。
INSERT INTO anime_works (source, syobocal_tid, position, annict_work_id, anilist_id, mal_id)
SELECT @source, t.syobocal_tid, t.position, NULLIF(t.annict_work_id, 0), NULLIF(t.anilist_id, 0), NULLIF(t.mal_id, 0)
FROM (
  SELECT unnest(@syobocal_tids::integer[]) AS syobocal_tid, unnest(@positions::smallint[]) AS position,
    unnest(@annict_work_ids::integer[]) AS annict_work_id, unnest(@anilist_ids::integer[]) AS anilist_id,
    unnest(@mal_ids::integer[]) AS mal_id
) AS t;

-- name: ListAnimeWorks :many
SELECT w.syobocal_tid, w.position, w.annict_work_id, w.anilist_id, w.mal_id
FROM anime_works w
WHERE w.syobocal_tid = ANY(@syobocal_tids::integer[])
ORDER BY w.syobocal_tid, w.position;

-- name: ListAnimeChannels :many
-- サービスごとに、取り込んだ対応と管理者の上書き、使う値を返す (設計書 3.8)。
-- 地上波は種別とサービス ID で、BS と CS はネットワーク ID とサービス ID で、取り込んだ対応と照らし合わせる。
SELECT s.id AS service_id,
  d.name AS definition_name,
  d.syobocal_channel_id AS imported_syobocal_channel_id,
  d.annict_channel_id AS imported_annict_channel_id,
  o.syobocal_channel_id AS override_syobocal_channel_id,
  o.annict_channel_id AS override_annict_channel_id,
  COALESCE(o.syobocal_channel_id, d.syobocal_channel_id) AS syobocal_channel_id,
  COALESCE(o.annict_channel_id, d.annict_channel_id) AS annict_channel_id
FROM services s
LEFT JOIN anime_channel_definitions d ON d.sid = s.sid AND d.channel_type = s.channel_type
  AND (d.network_id = s.network_id OR (d.channel_type = 'GR' AND d.network_id = 0))
LEFT JOIN anime_channel_overrides o ON o.service_id = s.id
ORDER BY s.id;

-- name: LockAnimeChannelOverride :one
-- 変える前の値を監査ログに残すため、上書きの行をロックして読む。サービスがなければ行を返さない。
SELECT s.id AS service_id, o.syobocal_channel_id, o.annict_channel_id
FROM services s LEFT JOIN anime_channel_overrides o ON o.service_id = s.id
WHERE s.id = @service_id
FOR UPDATE OF s;

-- name: UpsertAnimeChannelOverride :exec
INSERT INTO anime_channel_overrides (service_id, syobocal_channel_id, annict_channel_id, updated_at)
VALUES (@service_id, sqlc.narg(syobocal_channel_id), sqlc.narg(annict_channel_id), @now)
ON CONFLICT (service_id) DO UPDATE SET
  syobocal_channel_id = excluded.syobocal_channel_id, annict_channel_id = excluded.annict_channel_id, updated_at = excluded.updated_at;

-- name: DeleteAnimeChannelOverride :exec
DELETE FROM anime_channel_overrides WHERE service_id = @service_id;

-- name: GetEPGChangeVersion :one
-- EPG の変更の通知の、今の版数を返す。一度も通知していなければ 0。
SELECT COALESCE((SELECT version FROM change_versions WHERE channel = 'epg'), 0)::bigint AS version;

-- name: GetAnimeMatchCursor :one
SELECT epg_version FROM anime_match_cursor WHERE id = 1;

-- name: SaveAnimeMatchCursor :exec
INSERT INTO anime_match_cursor (id, epg_version, updated_at) VALUES (1, @epg_version, @now)
ON CONFLICT (id) DO UPDATE SET epg_version = excluded.epg_version, updated_at = excluded.updated_at;

-- name: ListAnimePrograms :many
-- サービス service_ids の、from から to までと放送時間が重なる、アニメと特撮 (ジャンルの大分類 0x7) の番組を返す。
SELECT p.service_id, p.event_id, p.start_at, p.end_at, p.name, m.finalized_at
FROM programs p
LEFT JOIN program_anime_matches m ON m.service_id = p.service_id AND m.start_at = p.start_at
WHERE p.service_id = ANY(@service_ids::bigint[]) AND p.start_at < @range_end AND p.end_at > @range_start
  AND p.genres @> '[{"lv1": 7}]'::jsonb
ORDER BY p.service_id, p.start_at;

-- name: ListAnimeProgramsToFinalize :many
-- 放送が終わり、まだアーカイブへ移していない、結びつきを確定させていないアニメと特撮の番組を返す。
SELECT p.service_id, p.event_id, p.start_at, p.end_at, p.name
FROM programs p
LEFT JOIN program_anime_matches m ON m.service_id = p.service_id AND m.start_at = p.start_at
WHERE p.service_id = ANY(@service_ids::bigint[]) AND p.end_at <= @now AND NOT p.end_undecided
  AND p.genres @> '[{"lv1": 7}]'::jsonb AND m.finalized_at IS NULL
ORDER BY p.service_id, p.start_at;

-- name: ListSyobocalLookups :many
SELECT channel_id, day_start, fetched_at FROM syobocal_lookups
WHERE channel_id = ANY(@channel_ids::integer[]) AND day_start >= @range_start AND day_start < @range_end;

-- name: UpsertSyobocalLookup :exec
INSERT INTO syobocal_lookups (channel_id, day_start, fetched_at) VALUES (@channel_id, @day_start, @now)
ON CONFLICT (channel_id, day_start) DO UPDATE SET fetched_at = excluded.fetched_at;

-- name: UpsertSyobocalProgram :exec
INSERT INTO syobocal_programs (pid, tid, channel_id, start_at, end_at, offset_seconds, count, sub_title, st_sub_title, flag, deleted, fetched_at)
VALUES (@pid, @tid, @channel_id, @start_at, @end_at, @offset_seconds, @count, @sub_title, @st_sub_title, @flag, @deleted, @now)
ON CONFLICT (pid) DO UPDATE SET
  tid = excluded.tid, channel_id = excluded.channel_id, start_at = excluded.start_at, end_at = excluded.end_at,
  offset_seconds = excluded.offset_seconds, count = excluded.count, sub_title = excluded.sub_title,
  st_sub_title = excluded.st_sub_title, flag = excluded.flag, deleted = excluded.deleted, fetched_at = excluded.fetched_at;

-- name: DeleteMissingSyobocalPrograms :exec
-- 引いた ChID と期間にあったのに、返ってこなかった放送時間データを消す。期間は db.php と同じく「from < 終了 AND 開始 < to」で決める。
DELETE FROM syobocal_programs
WHERE channel_id = ANY(@channel_ids::integer[]) AND end_at > @range_start AND start_at < @range_end AND pid <> ALL(@pids::integer[]);

-- name: ListSyobocalPrograms :many
SELECT * FROM syobocal_programs
WHERE channel_id = ANY(@channel_ids::integer[]) AND end_at > @range_start AND start_at < @range_end
ORDER BY channel_id, start_at, pid;

-- name: DeleteMovedProgramAnimeMatches :exec
-- 放送時間が変わった番組の、前の放送時間の結びつきを消す。アーカイブへ移した、同じイベント ID の番組の結びつきは残す。
DELETE FROM program_anime_matches m
WHERE m.service_id = @service_id AND m.event_id = @event_id AND m.start_at <> @start_at
  AND NOT EXISTS (
    SELECT 1 FROM program_archives a WHERE a.service_id = m.service_id AND a.event_id = m.event_id AND a.start_at = m.start_at
  );

-- name: UpsertProgramAnimeMatch :exec
-- 照合したことを記録する。確定させた日時は、一度書いたら消さない。
INSERT INTO program_anime_matches (service_id, start_at, event_id, matched_at, finalized_at)
VALUES (@service_id, @start_at, @event_id, @now, sqlc.narg(finalized_at))
ON CONFLICT (service_id, start_at) DO UPDATE SET
  event_id = excluded.event_id, matched_at = excluded.matched_at,
  finalized_at = COALESCE(program_anime_matches.finalized_at, excluded.finalized_at);

-- name: DeleteProgramAnimeEpisodes :exec
DELETE FROM program_anime_episodes WHERE service_id = @service_id AND start_at = @start_at;

-- name: InsertProgramAnimeEpisode :exec
INSERT INTO program_anime_episodes (
  service_id, start_at, position, source, syobocal_tid, syobocal_pid, count_start, count_end, subtitle, is_first, is_last, is_rerun
)
VALUES (
  @service_id, @start_at, @position, @source, sqlc.narg(syobocal_tid), sqlc.narg(syobocal_pid),
  sqlc.narg(count_start), sqlc.narg(count_end), sqlc.narg(subtitle), @is_first, @is_last, @is_rerun
);

-- name: ListProgramAnimeEpisodes :many
-- 番組 (サービスと開始時刻の組) に結びついた話を、番組ごとに並びの順で返す。
SELECT e.* FROM program_anime_episodes e
WHERE (e.service_id, e.start_at) IN (SELECT unnest(@service_ids::bigint[]), unnest(@start_ats::timestamptz[]))
ORDER BY e.service_id, e.start_at, e.position;

-- name: PruneSyobocalPrograms :execrows
DELETE FROM syobocal_programs WHERE end_at < @before;

-- name: PruneSyobocalLookups :execrows
DELETE FROM syobocal_lookups WHERE day_start < @before;

-- name: PruneProgramAnimeMatches :execrows
-- 番組表からも過去の番組からもなくなった番組の結びつきを消す。照合したばかりの結びつきは、番組を書く途中かもしれないので残す。
DELETE FROM program_anime_matches m
WHERE m.matched_at < @before
  AND NOT EXISTS (SELECT 1 FROM programs p WHERE p.service_id = m.service_id AND p.start_at = m.start_at)
  AND NOT EXISTS (SELECT 1 FROM program_archives a WHERE a.service_id = m.service_id AND a.start_at = m.start_at);

-- name: UpsertSyobocalTitle :exec
INSERT INTO syobocal_titles (tid, title, title_key, last_update) VALUES (@tid, @title, @title_key, sqlc.narg(last_update))
ON CONFLICT (tid) DO UPDATE SET title = excluded.title, title_key = excluded.title_key, last_update = excluded.last_update;

-- name: CountSyobocalTitles :one
SELECT count(*)::integer FROM syobocal_titles;

-- name: GetSyobocalTitlesLastUpdate :one
-- 取り込んだタイトルの、最も新しい更新日時を返す。1 つも取り込んでいなければ、行を返さない。
SELECT last_update FROM syobocal_titles WHERE last_update IS NOT NULL ORDER BY last_update DESC LIMIT 1;

-- name: ListSyobocalTitlesByKey :many
SELECT tid, title FROM syobocal_titles WHERE title_key = @title_key ORDER BY tid LIMIT 2;

-- name: ListProgramRecordingIDs :many
-- 番組 (サービスと開始時刻の組) を録った録画を返す。EPGStation から取り込んだ録画は、自分で照合するので除く。
SELECT r.id FROM recordings r
WHERE r.service_id = @service_id AND r.start_at = @start_at
  AND NOT EXISTS (SELECT 1 FROM import_sources s WHERE s.kind = 'recorded' AND s.target_id = r.id);

-- name: DeleteRecordingAnimeEpisodes :exec
DELETE FROM recording_anime_episodes WHERE recording_id = @recording_id;

-- name: UpsertRecordingAnimeMatch :exec
INSERT INTO recording_anime_matches (recording_id, matched_at) VALUES (@recording_id, @now)
ON CONFLICT (recording_id) DO UPDATE SET matched_at = excluded.matched_at;

-- name: CopyProgramAnimeEpisodesToRecording :execrows
-- 番組 (サービスと開始時刻の組) に結びついた話を、録画に写す。呼ぶ前に、録画の照合した記録を書き、前に写した話を消す。
INSERT INTO recording_anime_episodes (
  recording_id, position, source, syobocal_tid, syobocal_pid, count_start, count_end, subtitle, is_first, is_last, is_rerun
)
SELECT @recording_id, e.position, e.source, e.syobocal_tid, e.syobocal_pid, e.count_start, e.count_end, e.subtitle, e.is_first, e.is_last, e.is_rerun
FROM program_anime_episodes e
WHERE e.service_id = @service_id AND e.start_at = @start_at;

-- name: ProgramAnimeMatchExists :one
SELECT EXISTS (SELECT 1 FROM program_anime_matches WHERE service_id = @service_id AND start_at = @start_at);

-- name: InsertRecordingAnimeEpisode :exec
INSERT INTO recording_anime_episodes (
  recording_id, position, source, syobocal_tid, syobocal_pid, count_start, count_end, subtitle, is_first, is_last, is_rerun
)
VALUES (
  @recording_id, @position, @source, sqlc.narg(syobocal_tid), sqlc.narg(syobocal_pid),
  sqlc.narg(count_start), sqlc.narg(count_end), sqlc.narg(subtitle), @is_first, @is_last, @is_rerun
);

-- name: ListRecordingAnimeEpisodes :many
SELECT * FROM recording_anime_episodes WHERE recording_id = @recording_id ORDER BY position;

-- name: ListImportedRecordingsToMatch :many
-- EPGStation から取り込んだ、まだ照合していない録画を、古い順に max_count 件返す。
-- 取り込んだ録画はジャンルを持たないことがあるので、ジャンルのない録画も返す。アニメと特撮でないジャンルを持つ録画は返さない。
SELECT r.id, r.service_id, r.start_at, r.end_at, r.name FROM recordings r
JOIN import_sources s ON s.kind = 'recorded' AND s.target_id = r.id
LEFT JOIN recording_anime_matches m ON m.recording_id = r.id
WHERE m.recording_id IS NULL AND r.state <> 'recording'
  AND (r.genres = '[]'::jsonb OR r.genres @> '[{"lv1": 7}]'::jsonb)
ORDER BY r.start_at, r.id
LIMIT @max_count;
