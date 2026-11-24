-- name: GetGoogleApp :one
SELECT * FROM google_apps WHERE id = 1;

-- name: UpsertGoogleApp :exec
INSERT INTO google_apps (id, client_id, client_secret_encrypted, updated_at)
VALUES (1, @client_id, @client_secret_encrypted, @now)
ON CONFLICT (id) DO UPDATE SET
  client_id = EXCLUDED.client_id, client_secret_encrypted = EXCLUDED.client_secret_encrypted, updated_at = EXCLUDED.updated_at;

-- name: DeleteGoogleApp :execrows
DELETE FROM google_apps WHERE id = 1;

-- name: InsertGoogleAuthorization :exec
INSERT INTO google_authorizations (id, state_hash, session_id, user_id, purpose, code_verifier, redirect_uri, created_at, expires_at)
VALUES (@id, @state_hash, @session_id, @user_id, @purpose, @code_verifier, @redirect_uri, @now, @expires_at);

-- name: TakeGoogleAuthorization :one
-- 認可の途中の状態は 1 回しか使わせないので、引くと同時に消す。
DELETE FROM google_authorizations WHERE state_hash = @state_hash AND expires_at > @now
RETURNING *;

-- name: DeleteExpiredGoogleAuthorizations :execrows
DELETE FROM google_authorizations WHERE expires_at <= @now;

-- name: GetGoogleDriveConnection :one
SELECT * FROM google_drive_connections WHERE id = 1;

-- name: UpsertGoogleDriveConnection :exec
INSERT INTO google_drive_connections (id, refresh_token_encrypted, account_name, connected_at, revoked_at)
VALUES (1, @refresh_token_encrypted, @account_name, @now, NULL)
ON CONFLICT (id) DO UPDATE SET
  refresh_token_encrypted = EXCLUDED.refresh_token_encrypted, account_name = EXCLUDED.account_name,
  connected_at = EXCLUDED.connected_at, revoked_at = NULL;

-- name: MarkGoogleDriveConnectionRevoked :execrows
-- 取り消されたと分かったリフレッシュトークンの接続にだけマークを付ける。接続し直した後の行には付けない。
UPDATE google_drive_connections SET revoked_at = @now
WHERE id = 1 AND refresh_token_encrypted = @refresh_token_encrypted AND revoked_at IS NULL;

-- name: DeleteGoogleDriveConnection :one
DELETE FROM google_drive_connections WHERE id = 1
RETURNING *;

-- name: ListGoogleDriveFolders :many
SELECT * FROM google_drive_folders ORDER BY added_at, folder_id;

-- name: InsertGoogleDriveFolder :execrows
INSERT INTO google_drive_folders (folder_id, library_name, drive_id, name, added_at)
VALUES (@folder_id, @library_name, @drive_id, @name, @now)
ON CONFLICT (folder_id) DO NOTHING;

-- name: RenameGoogleDriveFolder :one
-- ライブラリの名前を変える。変える前の名前を返す。
UPDATE google_drive_folders AS f SET library_name = @library_name
FROM google_drive_folders AS old
WHERE f.folder_id = @folder_id AND old.folder_id = f.folder_id
RETURNING old.library_name;

-- name: GetGoogleDriveFolderByLibrary :one
SELECT * FROM google_drive_folders WHERE library_name = @library_name;

-- name: DeleteGoogleDriveFolder :one
DELETE FROM google_drive_folders WHERE folder_id = @folder_id
RETURNING *;

-- name: DeleteGoogleDriveFolders :execrows
DELETE FROM google_drive_folders;
