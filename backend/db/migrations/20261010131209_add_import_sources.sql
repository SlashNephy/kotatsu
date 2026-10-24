-- +goose Up
-- modify "recording_files" table
ALTER TABLE "recording_files" DROP CONSTRAINT "recording_files_kind_check", ADD CONSTRAINT "recording_files_kind_check" CHECK (kind = ANY (ARRAY['segment'::text, 'index'::text, 'thumbnail'::text, 'preview'::text, 'other'::text]));
-- create "import_sources" table
CREATE TABLE "import_sources" ("source" text NOT NULL, "kind" text NOT NULL, "source_id" bigint NOT NULL, "target_id" uuid NOT NULL, "created_at" timestamptz NOT NULL, PRIMARY KEY ("source", "kind", "source_id"));

-- +goose Down
-- reverse: create "import_sources" table
DROP TABLE "import_sources";
-- reverse: modify "recording_files" table
ALTER TABLE "recording_files" DROP CONSTRAINT "recording_files_kind_check", ADD CONSTRAINT "recording_files_kind_check" CHECK (kind = ANY (ARRAY['segment'::text, 'index'::text, 'thumbnail'::text, 'preview'::text]));
