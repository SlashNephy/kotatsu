-- name: CreatePlayer :exec
INSERT INTO players (id, user_id, session_id, state, started_at, last_seen_at)
VALUES (@id, @user_id, @session_id, @state, @now, @now);

-- name: UpdatePlayer :execrows
-- セッションのプレイヤーの状態を書き直す。ほかのセッションのプレイヤーは書き直さない。
UPDATE players SET state = @state, last_seen_at = @now WHERE id = @id AND session_id = @session_id;

-- name: DeletePlayer :execrows
DELETE FROM players WHERE id = @id AND session_id = @session_id;

-- name: DeleteStalePlayers :execrows
-- 閉じたとみなすプレイヤーを消す。操作も一緒に消える。
DELETE FROM players WHERE last_seen_at < @before;

-- name: ListPlayers :many
-- ユーザーの、閉じていないプレイヤーを、開いた順に返す。
SELECT p.id, p.session_id, s.user_agent, p.state, p.started_at, p.last_seen_at FROM players p
JOIN sessions s ON s.id = p.session_id
WHERE p.user_id = @user_id AND p.last_seen_at >= @alive_since
ORDER BY p.started_at, p.id;

-- name: GetPlayerOwner :one
-- プレイヤーのユーザーとセッション。閉じたとみなすプレイヤーは返さない。
SELECT user_id, session_id FROM players WHERE id = @id AND last_seen_at >= @alive_since;

-- name: CreatePlayerCommand :one
INSERT INTO player_commands (player_id, command, created_at) VALUES (@player_id, @command, @now) RETURNING id;

-- name: DeleteOldPlayerCommands :execrows
DELETE FROM player_commands WHERE created_at < @before;

-- name: ListPlayerCommands :many
-- プレイヤーへの操作のうち、after より新しいものを、届いた順に返す。
SELECT id, command FROM player_commands WHERE player_id = @player_id AND id > @after ORDER BY id;

-- name: LastPlayerCommandID :one
SELECT coalesce(max(id), 0)::bigint FROM player_commands WHERE player_id = @player_id;

-- name: BumpPlayerVersion :exec
INSERT INTO player_versions (user_id, version, updated_at) VALUES (@user_id, 1, @now)
ON CONFLICT (user_id) DO UPDATE SET version = player_versions.version + 1, updated_at = @now;

-- name: GetPlayerVersion :one
SELECT coalesce((SELECT version FROM player_versions WHERE user_id = @user_id), 0)::bigint;
