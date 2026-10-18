-- +goose Up
-- create "api_tokens" table
CREATE TABLE "api_tokens" ("id" uuid NOT NULL, "user_id" uuid NOT NULL, "name" text NOT NULL, "token_hash" bytea NOT NULL, "created_at" timestamptz NOT NULL, "last_used_at" timestamptz NULL, PRIMARY KEY ("id"), CONSTRAINT "api_tokens_token_hash_key" UNIQUE ("token_hash"), CONSTRAINT "api_tokens_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "api_tokens_user_id_idx" to table: "api_tokens"
CREATE INDEX "api_tokens_user_id_idx" ON "api_tokens" ("user_id");

-- +goose Down
-- reverse: create index "api_tokens_user_id_idx" to table: "api_tokens"
DROP INDEX "api_tokens_user_id_idx";
-- reverse: create "api_tokens" table
DROP TABLE "api_tokens";
