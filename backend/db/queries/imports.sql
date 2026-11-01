-- name: GetImportedTarget :one
-- 取り込み元のデータを、既に取り込んだかどうか。取り込んでいれば、作った行の ID を返す。
SELECT target_id FROM import_sources WHERE source = @source AND kind = @kind AND source_id = @source_id;

-- name: InsertImportSource :exec
INSERT INTO import_sources (source, kind, source_id, target_id, created_at)
VALUES (@source, @kind, @source_id, @target_id, @now);

-- name: ListServiceIDs :many
-- 取り込むデータが参照するサービスが、Kotatsu にあるかを確かめるのに使う。
SELECT id FROM services ORDER BY id;

-- name: InsertImportedRecording :exec
-- 取り込んだ、録画を終えた録画を記録する。予約とは結び付けない。
INSERT INTO recordings (id, reservation_id, service_id, event_id, name, description, name_normalized, description_normalized,
  start_at, end_at, state, failure_reason, protected, drops, errors, scrambled, recorded_from, recorded_until, created_at, updated_at)
VALUES (@id, NULL, @service_id, sqlc.narg(event_id), @name, @description, @name_normalized, @description_normalized,
  @start_at, @end_at, @state, NULL,
  @protected, @drops, @errors, @scrambled, @start_at, @end_at, @now, @now);

-- name: InsertImportedRecordingFile :exec
-- 取り込んだ録画のファイルを記録する。書き終えたファイルなので、大きさと終わりも記録する。
INSERT INTO recording_files (id, recording_id, kind, pool_id, path, size, started_at, ended_at, created_at)
VALUES (@id, @recording_id, @kind, @pool_id, @path, @size, @started_at, @ended_at, @now);
