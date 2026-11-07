-- +goose Up
-- create "calendar_feeds" table
CREATE TABLE "calendar_feeds" ("user_id" uuid NOT NULL, "token_hash" bytea NOT NULL, "created_at" timestamptz NOT NULL, "last_used_at" timestamptz NULL, PRIMARY KEY ("user_id"), CONSTRAINT "calendar_feeds_token_hash_key" UNIQUE ("token_hash"), CONSTRAINT "calendar_feeds_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);

-- +goose Down
-- reverse: create "calendar_feeds" table
DROP TABLE "calendar_feeds";
