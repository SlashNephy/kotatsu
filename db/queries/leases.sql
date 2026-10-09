-- name: AcquireLease :one
-- 期限の切れたリースだけを取れる。誰も持っていないか期限が切れていれば、世代番号を 1 つ上げて取る。
-- 他のインスタンスが期限内のリースを持っていれば、行を返さない。
INSERT INTO leases (name, holder, generation, acquired_at, renewed_at, expires_at)
VALUES (@name, @holder, 1, @now, @now, @expires_at)
ON CONFLICT (name) DO UPDATE
SET
  holder = EXCLUDED.holder,
  generation = leases.generation + 1,
  acquired_at = EXCLUDED.acquired_at,
  renewed_at = EXCLUDED.renewed_at,
  expires_at = EXCLUDED.expires_at
WHERE leases.expires_at <= EXCLUDED.acquired_at
RETURNING generation;

-- name: RenewLease :execrows
-- 同じ世代のリースを期限内に延長する。期限が切れていたり、他のインスタンスに取られていたりすれば、行を更新しない。
UPDATE leases
SET renewed_at = @now, expires_at = @expires_at
WHERE name = @name AND holder = @holder AND generation = @generation AND expires_at > @now;

-- name: ReleaseLease :exec
-- 期限をすぐに切れさせて、他のインスタンスがすぐに取れるようにする。世代番号は残す。
UPDATE leases
SET expires_at = @now
WHERE name = @name AND holder = @holder AND generation = @generation AND expires_at > @now;

-- name: LockLeaseForWrite :one
-- 書き込みのトランザクションの中で、まだ同じ世代のリースを持っていることを確かめる。
-- FOR SHARE で行をロックするので、このトランザクションが終わるまで、他のインスタンスはリースを取れない。
SELECT generation
FROM leases
WHERE name = @name AND holder = @holder AND generation = @generation AND expires_at > @now
FOR SHARE;

-- name: GetLease :one
SELECT * FROM leases WHERE name = @name;
