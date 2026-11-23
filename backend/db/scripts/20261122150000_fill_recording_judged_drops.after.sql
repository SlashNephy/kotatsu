-- 判定に数えるドロップの列 (judged_drops、#2076) を足す前の録画は、セグメントの PID ごとの内訳から、
-- 映像と音声のほかの PID のドロップを引いて埋める。内訳のないセグメント (内訳を記録する前に録ったものや、取り込んだ録画) と、
-- 映像と音声の PID が分からなかったセグメント (PMT を読めていない) の分は、すべて判定に数える。
UPDATE "recordings" AS r SET "judged_drops" = GREATEST(r."drops" - COALESCE((
  SELECT SUM((s ->> 'drops')::bigint)
  FROM "recording_files" AS f, jsonb_array_elements(f."pid_stats") AS s
  WHERE f."recording_id" = r."id" AND f."pid_stats" IS NOT NULL AND s ->> 'kind' NOT IN ('video', 'audio')
    AND EXISTS (SELECT 1 FROM jsonb_array_elements(f."pid_stats") AS m WHERE m ->> 'kind' IN ('video', 'audio'))
), 0), 0);
-- 映像と音声のほかの PID にしかドロップがなかった録画は、エラーありの完了から完了に直す (設計書 5.5)。
UPDATE "recordings" SET "state" = 'completed' WHERE "state" = 'completed_with_errors' AND "judged_drops" = 0;
