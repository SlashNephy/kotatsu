-- +goose Up
-- modify "recordings" table
ALTER TABLE "recordings" ADD COLUMN "judged_drops" bigint NOT NULL DEFAULT 0;
-- 今までの録画は、セグメントの PID ごとの内訳から、字幕・データ (other) の PID のドロップを引いて埋める。
-- 内訳のないセグメント (内訳を記録する前に録ったものや、取り込んだ録画) の分は、すべて判定に数える。
UPDATE "recordings" AS r SET "judged_drops" = GREATEST(r."drops" - COALESCE((
  SELECT SUM((s ->> 'drops')::bigint)
  FROM "recording_files" AS f, jsonb_array_elements(f."pid_stats") AS s
  WHERE f."recording_id" = r."id" AND f."pid_stats" IS NOT NULL AND s ->> 'kind' = 'other'
), 0), 0);
-- 字幕・データのドロップしかなかった録画は、エラーありの完了から完了に直す (設計書 5.5)。
UPDATE "recordings" SET "state" = 'completed' WHERE "state" = 'completed_with_errors' AND "judged_drops" = 0;

-- +goose Down
-- reverse: modify "recordings" table
ALTER TABLE "recordings" DROP COLUMN "judged_drops";
