-- +goose Up
-- create "sessions" table
CREATE TABLE "sessions" ("id" uuid NOT NULL, "token_hash" bytea NOT NULL, "user_id" uuid NOT NULL, "created_at" timestamptz NOT NULL, "authenticated_at" timestamptz NOT NULL, "last_seen_at" timestamptz NOT NULL, "expires_at" timestamptz NOT NULL, "revoked_at" timestamptz NULL, "user_agent" text NOT NULL, "source_ip" inet NULL, PRIMARY KEY ("id"), CONSTRAINT "sessions_token_hash_key" UNIQUE ("token_hash"), CONSTRAINT "sessions_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "sessions_user_id_idx" to table: "sessions"
CREATE INDEX "sessions_user_id_idx" ON "sessions" ("user_id");

-- +goose Down
-- reverse: create index "sessions_user_id_idx" to table: "sessions"
DROP INDEX "sessions_user_id_idx";
-- reverse: create "sessions" table
DROP TABLE "sessions";
