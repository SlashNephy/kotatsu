-- name: UpsertTerrestrialLineup :exec
-- 地上波のネットワークのサービスを書く (#1519)。版が同じでも、読んだ時刻は書き換える。
INSERT INTO terrestrial_lineups (network_id, transport_stream_id, version, services, observed_at)
VALUES (@network_id, @transport_stream_id, @version, @services, @observed_at)
ON CONFLICT (network_id) DO UPDATE SET
  transport_stream_id = excluded.transport_stream_id,
  version = excluded.version,
  services = excluded.services,
  observed_at = excluded.observed_at;

-- name: ListTerrestrialLineups :many
-- 読んだ地上波のネットワークのサービスを返す (#1519)。
SELECT network_id, transport_stream_id, version, services, observed_at
FROM terrestrial_lineups
ORDER BY network_id;
