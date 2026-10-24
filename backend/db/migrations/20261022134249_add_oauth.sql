-- +goose Up
-- modify "system_settings" table
ALTER TABLE "system_settings" ADD COLUMN "mcp_enabled" boolean NOT NULL DEFAULT false;
-- create "oauth_authorization_codes" table
CREATE TABLE "oauth_authorization_codes" ("code_hash" bytea NOT NULL, "user_id" uuid NOT NULL, "client_id" text NOT NULL, "client_name" text NOT NULL, "redirect_uri" text NOT NULL, "code_challenge" text NOT NULL, "scopes" text[] NOT NULL, "resource" text NOT NULL, "created_at" timestamptz NOT NULL, "expires_at" timestamptz NOT NULL, PRIMARY KEY ("code_hash"), CONSTRAINT "oauth_authorization_codes_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create "oauth_grants" table
CREATE TABLE "oauth_grants" ("id" uuid NOT NULL, "user_id" uuid NOT NULL, "client_id" text NOT NULL, "client_name" text NOT NULL, "scopes" text[] NOT NULL, "resource" text NOT NULL, "created_at" timestamptz NOT NULL, "last_used_at" timestamptz NULL, PRIMARY KEY ("id"), CONSTRAINT "oauth_grants_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "oauth_grants_user_id_idx" to table: "oauth_grants"
CREATE INDEX "oauth_grants_user_id_idx" ON "oauth_grants" ("user_id");
-- create "oauth_tokens" table
CREATE TABLE "oauth_tokens" ("token_hash" bytea NOT NULL, "grant_id" uuid NOT NULL, "kind" text NOT NULL, "created_at" timestamptz NOT NULL, "expires_at" timestamptz NOT NULL, "used_at" timestamptz NULL, PRIMARY KEY ("token_hash"), CONSTRAINT "oauth_tokens_grant_id_fkey" FOREIGN KEY ("grant_id") REFERENCES "oauth_grants" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "oauth_tokens_check" CHECK ((kind = 'refresh'::text) OR (used_at IS NULL)), CONSTRAINT "oauth_tokens_kind_check" CHECK (kind = ANY (ARRAY['access'::text, 'refresh'::text])));
-- create index "oauth_tokens_expires_at_idx" to table: "oauth_tokens"
CREATE INDEX "oauth_tokens_expires_at_idx" ON "oauth_tokens" ("expires_at");
-- create index "oauth_tokens_grant_id_idx" to table: "oauth_tokens"
CREATE INDEX "oauth_tokens_grant_id_idx" ON "oauth_tokens" ("grant_id");

-- +goose Down
-- reverse: create index "oauth_tokens_grant_id_idx" to table: "oauth_tokens"
DROP INDEX "oauth_tokens_grant_id_idx";
-- reverse: create index "oauth_tokens_expires_at_idx" to table: "oauth_tokens"
DROP INDEX "oauth_tokens_expires_at_idx";
-- reverse: create "oauth_tokens" table
DROP TABLE "oauth_tokens";
-- reverse: create index "oauth_grants_user_id_idx" to table: "oauth_grants"
DROP INDEX "oauth_grants_user_id_idx";
-- reverse: create "oauth_grants" table
DROP TABLE "oauth_grants";
-- reverse: create "oauth_authorization_codes" table
DROP TABLE "oauth_authorization_codes";
-- reverse: modify "system_settings" table
ALTER TABLE "system_settings" DROP COLUMN "mcp_enabled";
