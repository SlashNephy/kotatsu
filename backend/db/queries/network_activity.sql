-- name: AddNetworkActivity :exec
-- 集計を足し込む。書くまでの間に記録を無効にしていたら、書かない。
INSERT INTO network_activity (hour, purpose, host, initiator, success, count, sent_bytes, received_bytes)
SELECT t.hour, t.purpose, t.host, t.initiator, t.success, t.count, t.sent_bytes, t.received_bytes
FROM (
  SELECT
    unnest(@hours::timestamptz[]) AS hour, unnest(@purposes::text[]) AS purpose, unnest(@hosts::text[]) AS host,
    unnest(@initiators::text[]) AS initiator, unnest(@successes::boolean[]) AS success, unnest(@counts::bigint[]) AS count,
    unnest(@sent_bytes_list::bigint[]) AS sent_bytes, unnest(@received_bytes_list::bigint[]) AS received_bytes
) AS t
WHERE EXISTS (SELECT 1 FROM system_settings WHERE id = 1 AND network_activity_enabled)
ON CONFLICT (hour, purpose, host, initiator, success) DO UPDATE SET
  count = network_activity.count + EXCLUDED.count,
  sent_bytes = network_activity.sent_bytes + EXCLUDED.sent_bytes,
  received_bytes = network_activity.received_bytes + EXCLUDED.received_bytes;

-- name: DeleteAllNetworkActivity :exec
DELETE FROM network_activity;

-- name: DeleteNetworkActivityBefore :execrows
DELETE FROM network_activity WHERE hour < @before;

-- name: ListNetworkActivity :many
-- 新しい時間帯から、保持期間の中の集計を返す。
SELECT * FROM network_activity WHERE hour >= @since ORDER BY hour DESC, purpose, host, initiator, success DESC LIMIT @max_count;

-- name: SumNetworkActivity :one
SELECT COALESCE(SUM(count), 0)::bigint AS count, COALESCE(SUM(sent_bytes), 0)::bigint AS sent_bytes, COALESCE(SUM(received_bytes), 0)::bigint AS received_bytes
FROM network_activity WHERE hour >= @since;
