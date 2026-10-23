-- +goose Up
-- create "oidc_logins" table
CREATE TABLE "oidc_logins" ("id" uuid NOT NULL, "token_hash" bytea NOT NULL, "state" text NOT NULL, "nonce" text NOT NULL, "code_verifier" text NOT NULL, "redirect_uri" text NOT NULL, "created_at" timestamptz NOT NULL, "expires_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "oidc_logins_token_hash_key" UNIQUE ("token_hash"));
-- create index "oidc_logins_expires_at_idx" to table: "oidc_logins"
CREATE INDEX "oidc_logins_expires_at_idx" ON "oidc_logins" ("expires_at");
-- create "user_oidc_identities" table
CREATE TABLE "user_oidc_identities" ("id" uuid NOT NULL, "user_id" uuid NOT NULL, "issuer" text NOT NULL, "subject" text NOT NULL, "created_at" timestamptz NOT NULL, "last_used_at" timestamptz NULL, PRIMARY KEY ("id"), CONSTRAINT "user_oidc_identities_issuer_subject_key" UNIQUE ("issuer", "subject"), CONSTRAINT "user_oidc_identities_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "user_oidc_identities_user_id_idx" to table: "user_oidc_identities"
CREATE INDEX "user_oidc_identities_user_id_idx" ON "user_oidc_identities" ("user_id");

-- +goose Down
-- reverse: create index "user_oidc_identities_user_id_idx" to table: "user_oidc_identities"
DROP INDEX "user_oidc_identities_user_id_idx";
-- reverse: create "user_oidc_identities" table
DROP TABLE "user_oidc_identities";
-- reverse: create index "oidc_logins_expires_at_idx" to table: "oidc_logins"
DROP INDEX "oidc_logins_expires_at_idx";
-- reverse: create "oidc_logins" table
DROP TABLE "oidc_logins";
