-- name: LockOAuthDeviceCodes :exec
-- コードの数を数えてから足すまでを、ほかの発行と重ねない。重ねると、上限を超えて足せてしまう。
SELECT pg_advisory_xact_lock(hashtext('kotatsu.oauth_device_codes'));

-- name: CountValidOAuthDeviceCodes :one
-- 有効な (期限の前で、受け取られていない) コードの数を、全体と、接続元ごとに数える。
SELECT
  count(*) AS total,
  count(*) FILTER (WHERE source_network = @source_network) AS from_source
FROM oauth_device_codes
WHERE expires_at > @now AND used_at IS NULL;

-- name: ExpireOldestPendingOAuthDeviceCode :execrows
-- まだ承認していない有効なコードのうち、いちばん古いものを失効させる。source_network を渡せば、その接続元のものに限る。
-- 失効させたコードは、期限を過ぎたものと同じく、問い合わせに expired_token を返す。
UPDATE oauth_device_codes SET expires_at = @now
WHERE device_code_hash = (
  SELECT c.device_code_hash FROM oauth_device_codes c
  WHERE c.expires_at > @now AND c.used_at IS NULL AND c.approved_at IS NULL
    AND (sqlc.narg(source_network)::cidr IS NULL OR c.source_network = sqlc.narg(source_network)::cidr)
  ORDER BY c.created_at, c.device_code_hash
  LIMIT 1
);

-- name: CreateOAuthDeviceCode :exec
INSERT INTO oauth_device_codes (
  device_code_hash, user_code, client_id, device_name, scopes, resource, source_network, created_at, expires_at,
  last_polled_at, approved_by, approved_at, used_at
) VALUES (
  @device_code_hash, @user_code, @client_id, @device_name, @scopes, @resource, @source_network, @now, @expires_at,
  NULL, NULL, NULL, NULL
);

-- name: GetOAuthDeviceCodeByUserCode :one
-- 承認の画面に出す、有効なコードを引く。
SELECT device_name, user_code, approved_by FROM oauth_device_codes
WHERE user_code = @user_code AND expires_at > @now AND used_at IS NULL;

-- name: LockOAuthDeviceCodeByUserCode :one
-- 承認するコードを、ほかの承認と重ならないようロックして引く。
SELECT device_code_hash, device_name, client_id, approved_by FROM oauth_device_codes
WHERE user_code = @user_code AND expires_at > @now AND used_at IS NULL
FOR UPDATE;

-- name: ApproveOAuthDeviceCode :exec
UPDATE oauth_device_codes SET approved_by = @user_id, approved_at = @now WHERE device_code_hash = @device_code_hash;

-- name: LockOAuthDeviceCode :one
-- 端末の問い合わせのコードを、ほかの問い合わせと重ならないようロックして引く。
SELECT client_id, device_name, scopes, resource, expires_at, last_polled_at, approved_by, used_at
FROM oauth_device_codes
WHERE device_code_hash = @device_code_hash
FOR UPDATE;

-- name: GetOAuthDeviceApprover :one
-- コードを承認したユーザーの、今のロールと、無効にしたかを引く。
SELECT role, disabled_at FROM users WHERE id = @id;

-- name: TouchOAuthDeviceCodePoll :exec
UPDATE oauth_device_codes SET last_polled_at = @now WHERE device_code_hash = @device_code_hash;

-- name: MarkOAuthDeviceCodeUsed :exec
UPDATE oauth_device_codes SET used_at = @now WHERE device_code_hash = @device_code_hash;

-- name: DeleteExpiredOAuthDeviceCodes :execrows
-- 期限を過ぎてから時間の経ったコードと、受け取られたコードを消す。期限の直後は、問い合わせに expired_token を返せるよう残す。
DELETE FROM oauth_device_codes WHERE device_code_hash IN (
  SELECT c.device_code_hash FROM oauth_device_codes c
  WHERE c.expires_at <= @expired_before OR c.used_at <= @expired_before
  LIMIT @max_count
);

-- name: GetActiveOAuthDeviceGrantRole :one
-- 取り消していない端末の連携の、ユーザーの今のロールを引く。無効にしたユーザーの連携は返さない。
SELECT u.role FROM oauth_grants g
JOIN users u ON u.id = g.user_id
WHERE g.id = @id AND g.kind = 'device' AND g.revoked_at IS NULL AND u.disabled_at IS NULL;

-- name: RevokeOAuthDeviceGrant :exec
-- 端末の連携を取り消す。行は消さず、取り消した日時を書く。系統のトークンは、期限を過ぎてから消える。
UPDATE oauth_grants SET revoked_at = @now WHERE id = @id AND kind = 'device' AND revoked_at IS NULL;

-- name: RevokeUserOAuthDeviceGrant :one
-- ユーザーの端末の連携を取り消し、取り消した端末を返す。ほかのユーザーの連携と、取り消し済みの連携は取り消さない。
UPDATE oauth_grants SET revoked_at = @now
WHERE id = @id AND user_id = @user_id AND kind = 'device' AND revoked_at IS NULL
RETURNING client_id, client_name;
