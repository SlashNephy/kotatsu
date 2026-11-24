-- name: UpsertBroadcastLineup :exec
-- ネットワークの構成を書く (#1519)。バージョンが同じでも、読んだ時刻は書き換える。
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

-- name: UpsertBroadcastServiceActivity :exec
-- 衛星のサービスの番組の有無を書く (#2034)。
INSERT INTO broadcast_service_activities (network_id, service_id, transport_stream_id, has_events, observed_at)
VALUES (@network_id, @service_id, @transport_stream_id, @has_events, @observed_at)
ON CONFLICT (network_id, service_id) DO UPDATE SET
  transport_stream_id = excluded.transport_stream_id,
  has_events = excluded.has_events,
  observed_at = excluded.observed_at;

-- name: ListIdleBroadcastServices :many
-- 最後に読んだとき番組のなかった衛星のサービスを返す (#2034)。
SELECT network_id, service_id
FROM broadcast_service_activities
WHERE NOT has_events
ORDER BY network_id, service_id;
