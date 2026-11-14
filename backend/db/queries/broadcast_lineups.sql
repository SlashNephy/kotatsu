-- name: UpsertBroadcastLineup :exec
-- ネットワークの構成を書く (#1519)。版が同じでも、読んだ時刻は書き換える。
INSERT INTO broadcast_lineups (network_id, version, streams, observed_at)
VALUES (@network_id, @version, @streams, @observed_at)
ON CONFLICT (network_id) DO UPDATE SET
  version = excluded.version,
  streams = excluded.streams,
  observed_at = excluded.observed_at;

-- name: ListBroadcastLineups :many
-- 読んだ衛星のネットワークの構成を返す (#1519)。
SELECT network_id, version, streams, observed_at
FROM broadcast_lineups
ORDER BY network_id;
