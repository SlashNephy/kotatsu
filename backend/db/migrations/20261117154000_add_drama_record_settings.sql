-- +goose Up
-- create "drama_record_settings" table
CREATE TABLE "drama_record_settings" ("user_id" uuid NOT NULL, "threshold_percent" smallint NOT NULL DEFAULT 20, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("user_id"), CONSTRAINT "drama_record_settings_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "drama_record_settings_threshold_percent_check" CHECK ((threshold_percent >= 1) AND (threshold_percent <= 100)));

-- +goose Down
-- reverse: create "drama_record_settings" table
DROP TABLE "drama_record_settings";
