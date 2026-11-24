-- name: ListDramaRecordApps :many
SELECT * FROM drama_record_apps ORDER BY destination;

-- name: GetDramaRecordApp :one
SELECT * FROM drama_record_apps WHERE destination = @destination;

-- name: UpsertDramaRecordApp :exec
INSERT INTO drama_record_apps (destination, client_id, client_secret_encrypted, updated_at)
VALUES (@destination, @client_id, @client_secret_encrypted, @now)
ON CONFLICT (destination) DO UPDATE SET
  client_id = EXCLUDED.client_id, client_secret_encrypted = EXCLUDED.client_secret_encrypted, updated_at = EXCLUDED.updated_at;

-- name: DeleteDramaRecordApp :execrows
DELETE FROM drama_record_apps WHERE destination = @destination;

-- name: InsertDramaRecordAuthorization :exec
INSERT INTO drama_record_authorizations (id, state_hash, session_id, user_id, destination, code_verifier, redirect_uri, created_at, expires_at)
VALUES (@id, @state_hash, @session_id, @user_id, @destination, @code_verifier, @redirect_uri, @now, @expires_at);

-- name: TakeDramaRecordAuthorization :one
-- 認可の途中の状態は 1 回しか使わせないので、引くと同時に消す。
DELETE FROM drama_record_authorizations WHERE state_hash = @state_hash AND expires_at > @now
RETURNING *;

-- name: DeleteExpiredDramaRecordAuthorizations :execrows
DELETE FROM drama_record_authorizations WHERE expires_at <= @now;

-- name: ListDramaRecordConnections :many
SELECT * FROM drama_record_connections WHERE user_id = @user_id ORDER BY destination;

-- name: GetDramaRecordConnection :one
SELECT * FROM drama_record_connections WHERE user_id = @user_id AND destination = @destination;

-- name: LockDramaRecordConnection :one
-- トークンを更新するときは、ほかのプロセスが同じ連携を同時に更新しないよう、行をロックしてから読み直す。
-- Simkl は、更新すると前のアクセストークンをすぐに無効にするので、2 か所で更新すると、互いのトークンを無効にし合う。
SELECT * FROM drama_record_connections WHERE user_id = @user_id AND destination = @destination FOR UPDATE;

-- name: UpsertDramaRecordConnection :exec
-- 連携し直したら、トークンを入れ替え、取り込みの様子を初めからにする (別のアカウントと連携し直したかもしれないため)。
INSERT INTO drama_record_connections (
  user_id, destination, access_token_encrypted, refresh_token_encrypted, access_token_expires_at, account_name, connected_at
)
VALUES (@user_id, @destination, @access_token_encrypted, @refresh_token_encrypted, @access_token_expires_at, @account_name, @now)
ON CONFLICT (user_id, destination) DO UPDATE SET
  access_token_encrypted = EXCLUDED.access_token_encrypted, refresh_token_encrypted = EXCLUDED.refresh_token_encrypted,
  access_token_expires_at = EXCLUDED.access_token_expires_at, account_name = EXCLUDED.account_name,
  connected_at = EXCLUDED.connected_at, reauthorization_required = false,
  watchlist_activity = NULL, watchlist_removed_activity = NULL, watchlist_checked_at = NULL, watchlist_imported_at = NULL,
  watchlist_error = NULL, watchlist_retry_at = NULL;

-- name: UpdateDramaRecordTokens :exec
UPDATE drama_record_connections SET
  access_token_encrypted = @access_token_encrypted, refresh_token_encrypted = @refresh_token_encrypted,
  access_token_expires_at = @access_token_expires_at
WHERE user_id = @user_id AND destination = @destination;

-- name: RequireDramaRecordReauthorization :exec
-- 更新のトークンが断られたら、連携し直すまで、記録も取り込みもしない。
UPDATE drama_record_connections SET reauthorization_required = true
WHERE user_id = @user_id AND destination = @destination;

-- name: DeleteDramaRecordConnection :one
DELETE FROM drama_record_connections WHERE user_id = @user_id AND destination = @destination
RETURNING *;

-- name: ListDramaRecordTokens :many
-- ログの報告でマスクするため、全ユーザーのドラマの記録先のトークンを返す (#1521)。
SELECT access_token_encrypted, refresh_token_encrypted FROM drama_record_connections ORDER BY user_id, destination;

-- name: InsertDramaRecord :execrows
-- 同じユーザー、番組、話、記録先の組は、1 回だけ記録する。既にあれば何もしない。
INSERT INTO drama_records (
  id, user_id, service_id, start_at, destination, tmdb_series_id, season_number, episode_number, program_name, watched_at,
  state, created_at, updated_at
)
VALUES (
  @id, @user_id, @service_id, @start_at, @destination, @tmdb_series_id, @season_number, @episode_number, @program_name, @now,
  'pending', @now, @now
)
ON CONFLICT (user_id, service_id, start_at, destination, tmdb_series_id, season_number, episode_number) DO NOTHING;

-- name: ListPendingDramaRecords :many
SELECT * FROM drama_records
WHERE user_id = @user_id AND service_id = @service_id AND start_at = @start_at AND destination = @destination AND state = 'pending'
ORDER BY tmdb_series_id, season_number, episode_number;

-- name: FinishDramaRecord :exec
UPDATE drama_records SET state = @state, url = sqlc.narg(url), error = sqlc.narg(error), updated_at = @now
WHERE id = @id AND state = 'pending';

-- name: ClaimDramaWatchlistImport :one
-- ウォッチリストを取り込むと決めたら、問い合わせた日時を残す。画面を開くたびにジョブを入れ直さないため。
UPDATE drama_record_connections SET watchlist_checked_at = @now
WHERE user_id = @user_id AND destination = @destination
RETURNING *;

-- name: FinishDramaWatchlistImport :exec
UPDATE drama_record_connections SET
  watchlist_activity = @activity, watchlist_removed_activity = @removed_activity,
  watchlist_imported_at = @now, watchlist_error = NULL, watchlist_retry_at = NULL
WHERE user_id = @user_id AND destination = @destination;

-- name: FailDramaWatchlistImport :exec
UPDATE drama_record_connections SET watchlist_error = @error, watchlist_retry_at = @retry_at
WHERE user_id = @user_id AND destination = @destination;

-- name: GetDramaWatchlistImport :one
SELECT * FROM drama_watchlist_imports WHERE user_id = @user_id AND destination = @destination AND tmdb_series_id = @tmdb_series_id;

-- name: ListDramaWatchlistImportSimklIDs :many
SELECT tmdb_series_id, simkl_id FROM drama_watchlist_imports
WHERE user_id = @user_id AND destination = @destination AND simkl_id IS NOT NULL;

-- name: UpsertDramaWatchlistImport :exec
INSERT INTO drama_watchlist_imports (user_id, destination, tmdb_series_id, simkl_id, status, applied, updated_at)
VALUES (@user_id, @destination, @tmdb_series_id, sqlc.narg(simkl_id), @status, @applied, @now)
ON CONFLICT (user_id, destination, tmdb_series_id) DO UPDATE SET
  simkl_id = COALESCE(EXCLUDED.simkl_id, drama_watchlist_imports.simkl_id), status = EXCLUDED.status,
  applied = EXCLUDED.applied, updated_at = EXCLUDED.updated_at;

-- name: InsertDramaWatchlistImportIfAbsent :exec
-- Kotatsu から記録して、Simkl が作品をウォッチリストに入れたとき、その状態を読んだことにする (コピーしたことにする)。
-- 次の取り込みで、見ただけの作品を、見ている作品に足さないため。
INSERT INTO drama_watchlist_imports (user_id, destination, tmdb_series_id, simkl_id, status, applied, updated_at)
VALUES (@user_id, @destination, @tmdb_series_id, NULL, @status, true, @now)
ON CONFLICT (user_id, destination, tmdb_series_id) DO NOTHING;

-- name: DeleteDramaWatchlistImport :exec
DELETE FROM drama_watchlist_imports WHERE user_id = @user_id AND destination = @destination AND tmdb_series_id = @tmdb_series_id;

-- name: ListPendingDramaWatchlistImports :many
-- まだ見ている作品にコピーしていない作品を返す。番組か録画に結びついたかどうかも返す。
SELECT i.*, (EXISTS (SELECT 1 FROM program_drama_matches p WHERE p.tmdb_series_id = i.tmdb_series_id)
  OR EXISTS (SELECT 1 FROM recording_drama_matches r WHERE r.tmdb_series_id = i.tmdb_series_id))::boolean AS linked
FROM drama_watchlist_imports i
WHERE i.user_id = @user_id AND i.destination = @destination AND NOT i.applied
ORDER BY i.tmdb_series_id;

-- name: MarkDramaWatchlistImportApplied :exec
UPDATE drama_watchlist_imports SET applied = true
WHERE user_id = @user_id AND destination = @destination AND tmdb_series_id = @tmdb_series_id;

-- name: DeleteDramaWatchlistEntryIfStatus :execrows
-- 見ている作品を、状態が status のままのときだけ外す。Simkl からコピーした後に、Kotatsu で状態を変えた作品は外さない。
DELETE FROM drama_watchlist_entries WHERE user_id = @user_id AND tmdb_series_id = @tmdb_series_id AND status = @status;
