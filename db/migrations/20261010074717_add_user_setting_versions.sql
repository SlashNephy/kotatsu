-- +goose Up
-- create "user_setting_versions" table
CREATE TABLE "user_setting_versions" ("user_id" uuid NOT NULL, "version" bigint NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("user_id"), CONSTRAINT "user_setting_versions_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);

-- +goose Down
-- reverse: create "user_setting_versions" table
DROP TABLE "user_setting_versions";
