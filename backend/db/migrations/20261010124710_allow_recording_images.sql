-- +goose Up
-- modify "recording_files" table
ALTER TABLE "recording_files" DROP CONSTRAINT "recording_files_kind_check", ADD CONSTRAINT "recording_files_kind_check" CHECK (kind = ANY (ARRAY['segment'::text, 'index'::text, 'thumbnail'::text, 'preview'::text]));

-- +goose Down
-- reverse: modify "recording_files" table
ALTER TABLE "recording_files" DROP CONSTRAINT "recording_files_kind_check", ADD CONSTRAINT "recording_files_kind_check" CHECK (kind = ANY (ARRAY['segment'::text, 'index'::text]));
