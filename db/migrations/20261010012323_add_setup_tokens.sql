-- +goose Up
-- create "setup_tokens" table
CREATE TABLE "setup_tokens" ("token_hash" bytea NOT NULL, "created_at" timestamptz NOT NULL, PRIMARY KEY ("token_hash"));

-- +goose Down
-- reverse: create "setup_tokens" table
DROP TABLE "setup_tokens";
