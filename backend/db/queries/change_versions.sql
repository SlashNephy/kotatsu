-- name: BumpChangeVersion :one
INSERT INTO change_versions (channel, version, updated_at)
VALUES (@channel, 1, @now)
ON CONFLICT (channel) DO UPDATE
SET version = change_versions.version + 1, updated_at = EXCLUDED.updated_at
RETURNING version;

-- name: NotifyChange :exec
-- NOTIFY はトランザクションのコミット時に送られる。ロールバックすれば送られない。
SELECT pg_notify(@channel::text, @payload::text);

-- name: ListChangeVersions :many
SELECT channel, version FROM change_versions WHERE channel = ANY(@channels::text[]);
