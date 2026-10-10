-- name: LockSetup :exec
-- 初回セットアップを、全インスタンスで 1 つずつ行わせる。トランザクションが終わるまでロックする。
SELECT pg_advisory_xact_lock(hashtext('kotatsu.setup'));

-- name: HasAdmin :one
-- 無効にした管理者も数える。管理者を作った後は、初回セットアップを開き直さない (設計書 9.2)。
SELECT EXISTS (SELECT 1 FROM users WHERE role = 'admin');

-- name: InsertSetupToken :exec
INSERT INTO setup_tokens (token_hash, created_at) VALUES (@token_hash, @now);

-- name: SetupTokenExists :one
SELECT EXISTS (SELECT 1 FROM setup_tokens WHERE token_hash = @token_hash);

-- name: DeleteSetupTokens :exec
DELETE FROM setup_tokens;
