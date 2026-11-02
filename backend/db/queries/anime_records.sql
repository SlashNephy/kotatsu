-- name: ListAnimeRecordApps :many
SELECT * FROM anime_record_apps ORDER BY destination;

-- name: GetAnimeRecordApp :one
SELECT * FROM anime_record_apps WHERE destination = @destination;

-- name: UpsertAnimeRecordApp :exec
INSERT INTO anime_record_apps (destination, client_id, client_secret_encrypted, updated_at)
VALUES (@destination, @client_id, @client_secret_encrypted, @now)
ON CONFLICT (destination) DO UPDATE SET
  client_id = EXCLUDED.client_id, client_secret_encrypted = EXCLUDED.client_secret_encrypted, updated_at = EXCLUDED.updated_at;

-- name: DeleteAnimeRecordApp :execrows
DELETE FROM anime_record_apps WHERE destination = @destination;

-- name: InsertAnimeRecordAuthorization :exec
INSERT INTO anime_record_authorizations (id, state_hash, session_id, user_id, destination, code_verifier, redirect_uri, created_at, expires_at)
VALUES (@id, @state_hash, @session_id, @user_id, @destination, @code_verifier, @redirect_uri, @now, @expires_at);

-- name: TakeAnimeRecordAuthorization :one
-- 認可の途中の状態は 1 回しか使わせないので、引くと同時に消す。
DELETE FROM anime_record_authorizations WHERE state_hash = @state_hash AND expires_at > @now
RETURNING *;

-- name: DeleteExpiredAnimeRecordAuthorizations :execrows
DELETE FROM anime_record_authorizations WHERE expires_at <= @now;

-- name: ListAnimeRecordConnections :many
SELECT * FROM anime_record_connections WHERE user_id = @user_id ORDER BY destination;

-- name: GetAnimeRecordConnection :one
SELECT * FROM anime_record_connections WHERE user_id = @user_id AND destination = @destination;

-- name: UpsertAnimeRecordConnection :exec
INSERT INTO anime_record_connections (user_id, destination, access_token_encrypted, account_name, connected_at)
VALUES (@user_id, @destination, @access_token_encrypted, @account_name, @now)
ON CONFLICT (user_id, destination) DO UPDATE SET
  access_token_encrypted = EXCLUDED.access_token_encrypted, account_name = EXCLUDED.account_name, connected_at = EXCLUDED.connected_at;

-- name: DeleteAnimeRecordConnection :one
DELETE FROM anime_record_connections WHERE user_id = @user_id AND destination = @destination
RETURNING *;

-- name: GetAnimeRecordSettings :one
SELECT * FROM anime_record_settings WHERE user_id = @user_id;

-- name: UpsertAnimeRecordSettings :one
INSERT INTO anime_record_settings (user_id, threshold_percent, watching_on_first, watching_on_episode, watched_on_last, updated_at)
VALUES (@user_id, @threshold_percent, @watching_on_first, @watching_on_episode, @watched_on_last, @now)
ON CONFLICT (user_id) DO UPDATE SET
  threshold_percent = EXCLUDED.threshold_percent, watching_on_first = EXCLUDED.watching_on_first,
  watching_on_episode = EXCLUDED.watching_on_episode, watched_on_last = EXCLUDED.watched_on_last, updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: ListLiveProgramsInRange :many
-- サービス service_id で、from から to までに重なる番組を、開始時刻の順に返す。終了時刻が未定の番組は、長さが分からないので返さない。
SELECT service_id, start_at, end_at, name FROM programs
WHERE service_id = @service_id AND start_at < @until AND end_at > @since AND NOT end_undecided
ORDER BY start_at;

-- name: GetRecordingProgram :one
SELECT id, service_id, start_at, end_at, name FROM recordings WHERE id = @id;

-- name: LockAnimeWatchProgress :one
-- 見た区間を足すときは、同じ番組への報告が重ならないよう、行を作ってからロックする。
INSERT INTO anime_watch_progress (user_id, service_id, start_at, played, played_ms, updated_at)
VALUES (@user_id, @service_id, @start_at, '[]', 0, @now)
ON CONFLICT (user_id, service_id, start_at) DO UPDATE SET updated_at = anime_watch_progress.updated_at
RETURNING played;

-- name: UpdateAnimeWatchProgress :exec
UPDATE anime_watch_progress SET played = @played, played_ms = @played_ms, updated_at = @now
WHERE user_id = @user_id AND service_id = @service_id AND start_at = @start_at;

-- name: PruneAnimeWatchProgress :execrows
DELETE FROM anime_watch_progress WHERE updated_at < @before;

-- name: InsertAnimeRecord :execrows
-- 同じユーザー、番組、話、記録先の組は、1 回だけ記録する。既にあれば何もしない。
-- 番組によらず手で記録した話 (service_id が 0。#1232) と同じ話は、記録できなかったもののほかにあれば、番組を見ても記録しない。
-- 手で記録するときも、同じ話の記録があれば記録しない。
INSERT INTO anime_records (
  id, user_id, service_id, start_at, destination, syobocal_tid, episode_number, subtitle, is_first, is_last,
  program_name, state, created_at, updated_at
)
SELECT @id, @user_id, @service_id, @start_at, @destination, @syobocal_tid, sqlc.narg(episode_number), sqlc.narg(subtitle), @is_first, @is_last,
  @program_name, 'pending', @now, @now
WHERE NOT EXISTS (
  SELECT 1 FROM anime_records r
  WHERE r.user_id = @user_id AND r.destination = @destination AND r.syobocal_tid = @syobocal_tid
    AND r.episode_number IS NOT DISTINCT FROM sqlc.narg(episode_number)::double precision AND r.state <> 'failed'
    AND (r.service_id = 0 OR @service_id::bigint = 0)
)
ON CONFLICT (user_id, service_id, start_at, destination, syobocal_tid, episode_number) DO NOTHING;

-- name: ListPendingAnimeRecords :many
SELECT * FROM anime_records
WHERE user_id = @user_id AND service_id = @service_id AND start_at = @start_at AND destination = @destination AND state = 'pending'
ORDER BY syobocal_tid, episode_number NULLS FIRST;

-- name: FinishAnimeRecord :exec
UPDATE anime_records SET state = @state, url = sqlc.narg(url), error = sqlc.narg(error), updated_at = @now
WHERE id = @id AND state = 'pending';
