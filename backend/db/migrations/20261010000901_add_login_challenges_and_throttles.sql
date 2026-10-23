-- +goose Up
-- create "login_throttles" table
CREATE TABLE "login_throttles" ("key" text NOT NULL, "failures" integer NOT NULL, "locked_until" timestamptz NOT NULL, "last_failed_at" timestamptz NOT NULL, PRIMARY KEY ("key"));
-- create "login_challenges" table
CREATE TABLE "login_challenges" ("id" uuid NOT NULL, "token_hash" bytea NOT NULL, "user_id" uuid NOT NULL, "created_at" timestamptz NOT NULL, "expires_at" timestamptz NOT NULL, "user_agent" text NOT NULL, "source_ip" inet NULL, PRIMARY KEY ("id"), CONSTRAINT "login_challenges_token_hash_key" UNIQUE ("token_hash"), CONSTRAINT "login_challenges_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "login_challenges_user_id_idx" to table: "login_challenges"
CREATE INDEX "login_challenges_user_id_idx" ON "login_challenges" ("user_id");

-- +goose Down
-- reverse: create index "login_challenges_user_id_idx" to table: "login_challenges"
DROP INDEX "login_challenges_user_id_idx";
-- reverse: create "login_challenges" table
DROP TABLE "login_challenges";
-- reverse: create "login_throttles" table
DROP TABLE "login_throttles";
