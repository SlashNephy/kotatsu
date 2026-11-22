-- name: EnsureChannelType :exec
-- 種別が無ければ既定値で足す。管理者が変えた表示名と並び順は上書きしない。
INSERT INTO channel_types (type, display_name, sort_order)
VALUES (@type, @display_name, @sort_order)
ON CONFLICT (type) DO NOTHING;

-- name: ListServices :many
SELECT id, network_id, sid, name, service_type, channel_type, channel, remote_control_key_id, logo_id
FROM services
ORDER BY id;

-- name: UpdateServicesPresence :execrows
-- 上流のサービスの一覧 (present) と突き合わせ、上流に無いサービスに、無いと気づいた時刻を書き、戻ってきたサービスの時刻を消す (#1973)。
-- 前から無いサービスの時刻は、初めて気づいた時刻のまま残す。変わった行の数を返す。
UPDATE services
SET gone_at = CASE WHEN id = ANY(@present::bigint[]) THEN NULL ELSE @now::timestamptz END
WHERE (id = ANY(@present::bigint[])) = (gone_at IS NOT NULL);

-- name: UpsertService :exec
-- 上流から来る項目だけを更新する。非表示の印と並び順の上書きは、管理者のものなので触らない。
INSERT INTO services (
  id, network_id, sid, name, name_normalized, service_type, channel_type, channel,
  remote_control_key_id, logo_id, created_at, updated_at
)
VALUES (
  @id, @network_id, @sid, @name, @name_normalized, @service_type, @channel_type, @channel,
  @remote_control_key_id, @logo_id, @now, @now
)
ON CONFLICT (id) DO UPDATE SET
  network_id = excluded.network_id,
  sid = excluded.sid,
  name = excluded.name,
  name_normalized = excluded.name_normalized,
  service_type = excluded.service_type,
  channel_type = excluded.channel_type,
  channel = excluded.channel,
  remote_control_key_id = excluded.remote_control_key_id,
  logo_id = excluded.logo_id,
  updated_at = excluded.updated_at;

-- name: ListPrograms :many
SELECT service_id, event_id, mirakurun_id, start_at, end_at, end_undecided, is_free, name, description,
  extended, genres, related_items, audios, video_resolution, series_episode, series_repeat
FROM programs
ORDER BY service_id, event_id;

-- name: ListProgramsOfService :many
SELECT service_id, event_id, mirakurun_id, start_at, end_at, end_undecided, is_free, name, description,
  extended, genres, related_items, audios, video_resolution, series_episode, series_repeat
FROM programs
WHERE service_id = @service_id
ORDER BY event_id;

-- name: UpsertPrograms :batchexec
INSERT INTO programs (
  service_id, event_id, mirakurun_id, start_at, end_at, end_undecided, is_free,
  name, description, name_normalized, description_normalized, extended_normalized, extended, genres, related_items, audios,
  video_resolution, series_episode, series_repeat, series, episode_number, search_grams, created_at, updated_at
)
VALUES (
  @service_id, @event_id, @mirakurun_id, @start_at, @end_at, @end_undecided, @is_free,
  @name, @description, @name_normalized, @description_normalized, @extended_normalized, @extended, @genres, @related_items, @audios,
  sqlc.narg(video_resolution), sqlc.narg(series_episode), sqlc.narg(series_repeat), @series::text, sqlc.narg(episode_number)::integer,
  sqlc.narg(search_grams)::text[], @now, @now
)
ON CONFLICT (service_id, event_id) DO UPDATE SET
  mirakurun_id = excluded.mirakurun_id,
  start_at = excluded.start_at,
  end_at = excluded.end_at,
  end_undecided = excluded.end_undecided,
  is_free = excluded.is_free,
  name = excluded.name,
  description = excluded.description,
  name_normalized = excluded.name_normalized,
  description_normalized = excluded.description_normalized,
  extended_normalized = excluded.extended_normalized,
  extended = excluded.extended,
  genres = excluded.genres,
  related_items = excluded.related_items,
  audios = excluded.audios,
  video_resolution = excluded.video_resolution,
  series_episode = excluded.series_episode,
  series_repeat = excluded.series_repeat,
  series = excluded.series,
  episode_number = excluded.episode_number,
  -- 番組名と概要が変わらない番組は、gram を NULL で送り、今の gram を残す (#966)。同じ配列を書き直すと、GIN の索引も書き直すからである。
  search_grams = COALESCE(excluded.search_grams, programs.search_grams),
  updated_at = excluded.updated_at;

-- name: ListProgramsWithoutSeries :many
-- シリーズ名と話数の列 (#1564) を埋めていない番組を返す。列を足す前に書いた番組と、列を知らない古いバイナリが書いた番組。
SELECT service_id, event_id, name, description_normalized, extended_normalized, series_episode
FROM programs
WHERE series IS NULL
ORDER BY service_id, event_id
LIMIT @batch_size::integer;

-- name: SetProgramSeries :batchexec
-- 番組の、番組名から読んだシリーズ名と話数 (#1564) を書く。ほかの列と更新日時は変えない。
UPDATE programs SET series = @series::text, episode_number = sqlc.narg(episode_number)::integer
WHERE service_id = @service_id AND event_id = @event_id;

-- name: ListProgramsWithoutSearchGrams :many
-- 番組の検索の gram の列 (#966) を埋めていない番組を返す。列を足す前に書いた番組と、列を知らない古いバイナリが書いた番組。
SELECT service_id, event_id, name_normalized, description_normalized
FROM programs
WHERE search_grams IS NULL
ORDER BY service_id, event_id
LIMIT @batch_size::integer;

-- name: SetProgramSearchGrams :batchexec
-- 番組の検索の gram (#966) を書く。ほかの列と更新日時は変えない。
UPDATE programs SET search_grams = @search_grams::text[]
WHERE service_id = @service_id AND event_id = @event_id;

-- name: ListProgramArchivesWithoutSearchGrams :many
-- 番組の検索の gram の列 (#966) を埋めていないアーカイブを返す。列を足す前に移した番組。
SELECT service_id, event_id, start_at, name_normalized, description_normalized
FROM program_archives
WHERE search_grams IS NULL
ORDER BY service_id, event_id, start_at
LIMIT @batch_size::integer;

-- name: SetProgramArchiveSearchGrams :batchexec
-- アーカイブの番組の検索の gram (#966) を書く。ほかの列は変えない。
UPDATE program_archives SET search_grams = @search_grams::text[]
WHERE service_id = @service_id AND event_id = @event_id AND start_at = @start_at;

-- name: DeletePrograms :batchexec
-- 放送が始まった番組は、アーカイブに移すまで残す (設計書 3.3、3.4)。
DELETE FROM programs WHERE service_id = @service_id AND event_id = @event_id AND start_at > @now;

-- name: InsertEPGChange :exec
INSERT INTO epg_changes (version, service_id, start_at, end_at, created_at)
VALUES (@version, @service_id, @start_at, @end_at, @created_at);

-- name: PruneEPGChanges :execrows
DELETE FROM epg_changes WHERE created_at < @before;

-- name: ListEPGChangesSince :many
SELECT version, service_id, start_at, end_at FROM epg_changes WHERE version > @version ORDER BY version, service_id;

-- name: InsertProgramRewrites :batchexec
INSERT INTO program_rewrites (service_id, event_id, start_at, field, before, after, observed_at)
VALUES (@service_id, @event_id, @start_at, @field, @before, @after, @observed_at);

-- name: PruneProgramRewrites :execrows
DELETE FROM program_rewrites WHERE observed_at < @before;

-- name: ServiceExists :one
SELECT EXISTS (SELECT 1 FROM services WHERE id = @id);

-- name: ArchivePrograms :execrows
-- 放送の終わりが before より前の番組を、最大 batch_size 件だけ program_archives へ移す。
-- 終了時刻が未定の番組は、開始時刻が before より前なら移す。猶予 (既定 24 時間) より長い番組は無いからである。
WITH moved AS (
  DELETE FROM programs
  WHERE (service_id, event_id) IN (
    SELECT p.service_id, p.event_id FROM programs p
    WHERE (NOT p.end_undecided AND p.end_at <= @before) OR (p.end_undecided AND p.start_at <= @before)
    ORDER BY p.end_at
    LIMIT @batch_size::integer
  )
  RETURNING *
)
INSERT INTO program_archives (
  service_id, event_id, mirakurun_id, start_at, end_at, end_undecided, is_free,
  name, description, name_normalized, description_normalized, extended_normalized, extended, genres, related_items, audios,
  video_resolution, series_episode, series_repeat, search_grams, created_at, updated_at, archived_at
)
SELECT
  service_id, event_id, mirakurun_id, start_at, end_at, end_undecided, is_free,
  name, description, name_normalized, description_normalized, extended_normalized, extended, genres, related_items, audios,
  video_resolution, series_episode, series_repeat, search_grams, created_at, updated_at, @now
FROM moved
-- 同じ番組を移し直すことは通常ないが、移した後に上流から同じ番組が届き、また移すときは新しい内容で上書きする。
ON CONFLICT (service_id, event_id, start_at) DO UPDATE SET
  mirakurun_id = excluded.mirakurun_id,
  end_at = excluded.end_at,
  end_undecided = excluded.end_undecided,
  is_free = excluded.is_free,
  name = excluded.name,
  description = excluded.description,
  name_normalized = excluded.name_normalized,
  description_normalized = excluded.description_normalized,
  extended_normalized = excluded.extended_normalized,
  extended = excluded.extended,
  genres = excluded.genres,
  related_items = excluded.related_items,
  audios = excluded.audios,
  video_resolution = excluded.video_resolution,
  series_episode = excluded.series_episode,
  series_repeat = excluded.series_repeat,
  search_grams = excluded.search_grams,
  updated_at = excluded.updated_at,
  archived_at = excluded.archived_at;

-- name: PruneProgramArchives :execrows
-- 放送の終わりが before より前のアーカイブを、最大 batch_size 件だけ消す。一度に大量の行をロックしないよう、少しずつ消す。
DELETE FROM program_archives
WHERE (service_id, event_id, start_at) IN (
  SELECT a.service_id, a.event_id, a.start_at FROM program_archives a
  WHERE a.end_at < @before
  ORDER BY a.end_at
  LIMIT @batch_size::integer
);
