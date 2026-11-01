-- +goose Up
-- arm-supplementary は取り込まなくなった。取り込んだ対応と、取り込んだ様子を消す。
DELETE FROM "anime_works" WHERE "source" = 'arm_supplementary';
DELETE FROM "anime_imports" WHERE "source" = 'arm_supplementary';
-- modify "anime_imports" table
ALTER TABLE "anime_imports" DROP CONSTRAINT "anime_imports_source_check", ADD CONSTRAINT "anime_imports_source_check" CHECK (source = ANY (ARRAY['saya_definitions'::text, 'arm'::text, 'syobocal_titles'::text]));
-- modify "anime_works" table
ALTER TABLE "anime_works" DROP CONSTRAINT "anime_works_source_check", ADD CONSTRAINT "anime_works_source_check" CHECK (source = 'arm'::text);

-- +goose Down
-- reverse: modify "anime_works" table
ALTER TABLE "anime_works" DROP CONSTRAINT "anime_works_source_check", ADD CONSTRAINT "anime_works_source_check" CHECK (source = ANY (ARRAY['arm'::text, 'arm_supplementary'::text]));
-- reverse: modify "anime_imports" table
ALTER TABLE "anime_imports" DROP CONSTRAINT "anime_imports_source_check", ADD CONSTRAINT "anime_imports_source_check" CHECK (source = ANY (ARRAY['saya_definitions'::text, 'arm'::text, 'arm_supplementary'::text, 'syobocal_titles'::text]));
