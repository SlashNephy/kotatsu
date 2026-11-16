-- +goose Up
-- create "tmdb_tokens" table
CREATE TABLE "tmdb_tokens" ("id" smallint NOT NULL, "token_encrypted" bytea NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "tmdb_tokens_id_check" CHECK (id = 1));

-- +goose Down
-- reverse: create "tmdb_tokens" table
DROP TABLE "tmdb_tokens";
