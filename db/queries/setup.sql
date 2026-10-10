-- name: LockSetup :exec
-- 初回セットアップを、全インスタンスで 1 つずつ行わせる。トランザクションが終わるまでロックする。
SELECT pg_advisory_xact_lock(hashtext('kotatsu.setup'));

-- name: HasActiveAdmin :one
SELECT EXISTS (SELECT 1 FROM users WHERE role = 'admin' AND disabled_at IS NULL);

-- name: InsertSetupToken :exec
INSERT INTO setup_tokens (token_hash, created_at) VALUES (@token_hash, @now);

-- name: SetupTokenExists :one
SELECT EXISTS (SELECT 1 FROM setup_tokens WHERE token_hash = @token_hash);

-- name: DeleteSetupTokens :exec
DELETE FROM setup_tokens;
