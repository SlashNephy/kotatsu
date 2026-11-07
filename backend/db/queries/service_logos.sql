-- name: UpsertServiceLogo :execrows
-- 版か画像が変わったときだけ書き換える (#1308)。
INSERT INTO service_logos (network_id, logo_id, logo_type, version, data, updated_at)
VALUES (@network_id, @logo_id, @logo_type, @version, @data, @now)
ON CONFLICT (network_id, logo_id, logo_type) DO UPDATE SET
  version = excluded.version,
  data = excluded.data,
  updated_at = excluded.updated_at
WHERE service_logos.version <> excluded.version OR service_logos.data <> excluded.data;

-- name: GetServiceLogo :one
-- サービスのロゴのうち、いちばん望ましい種類のものを返す (#1308)。HD の大きいロゴ (64×36) を先にする。
SELECT l.data
FROM services s
JOIN service_logos l ON l.network_id = s.network_id AND l.logo_id = s.logo_id
WHERE s.id = @service_id
ORDER BY array_position(ARRAY[5, 2, 4, 1, 3, 0]::smallint[], l.logo_type) NULLS LAST
LIMIT 1;
