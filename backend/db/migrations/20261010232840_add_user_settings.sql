-- +goose Up
-- create "user_settings" table
CREATE TABLE "user_settings" ("user_id" uuid NOT NULL, "jikkyo_enabled" boolean NOT NULL DEFAULT true, "captions_enabled" boolean NOT NULL DEFAULT true, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("user_id"), CONSTRAINT "user_settings_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);

-- +goose Down
-- reverse: create "user_settings" table
DROP TABLE "user_settings";
