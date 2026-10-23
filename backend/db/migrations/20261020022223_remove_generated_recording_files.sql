-- +goose Up
-- 索引、代表画像、プレビュー画像は、DB に記録せず、データディレクトリにセグメントのファイルの ID で置く (#883)。
DELETE FROM "recording_files" WHERE "kind" IN ('index', 'thumbnail', 'preview');
-- modify "recording_files" table
ALTER TABLE "recording_files" DROP CONSTRAINT "recording_files_kind_check", ADD CONSTRAINT "recording_files_kind_check" CHECK (kind = ANY (ARRAY['segment'::text, 'other'::text]));

-- +goose Down
-- reverse: modify "recording_files" table
ALTER TABLE "recording_files" DROP CONSTRAINT "recording_files_kind_check", ADD CONSTRAINT "recording_files_kind_check" CHECK (kind = ANY (ARRAY['segment'::text, 'index'::text, 'thumbnail'::text, 'preview'::text, 'other'::text]));
