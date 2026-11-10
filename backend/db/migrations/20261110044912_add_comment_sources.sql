-- +goose Up
-- 今の実況の設定を、NX-Jikkyo のソースのオプトインとして引き継ぐ (#1405)。
-- modify "system_settings" table
ALTER TABLE "system_settings" ADD COLUMN "comment_sources" text[] NOT NULL DEFAULT '{}';
UPDATE "system_settings" SET "comment_sources" = ARRAY['nx_jikkyo'] WHERE "jikkyo_enabled";
ALTER TABLE "system_settings" DROP COLUMN "jikkyo_enabled";
-- modify "user_settings" table
ALTER TABLE "user_settings" ADD COLUMN "comment_sources" text[] NOT NULL DEFAULT '{}';
UPDATE "user_settings" SET "comment_sources" = ARRAY['nx_jikkyo'] WHERE "jikkyo_enabled";
ALTER TABLE "user_settings" DROP COLUMN "jikkyo_enabled";

-- +goose Down
-- reverse: modify "user_settings" table
ALTER TABLE "user_settings" ADD COLUMN "jikkyo_enabled" boolean NOT NULL DEFAULT false;
UPDATE "user_settings" SET "jikkyo_enabled" = 'nx_jikkyo' = ANY("comment_sources");
ALTER TABLE "user_settings" DROP COLUMN "comment_sources";
-- reverse: modify "system_settings" table
ALTER TABLE "system_settings" ADD COLUMN "jikkyo_enabled" boolean NOT NULL DEFAULT false;
UPDATE "system_settings" SET "jikkyo_enabled" = 'nx_jikkyo' = ANY("comment_sources");
ALTER TABLE "system_settings" DROP COLUMN "comment_sources";
